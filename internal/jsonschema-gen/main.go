// Command jsonschema-gen emits the JSON Schema for pkg/config's file envelope.
// Regenerate with:
//
//	go generate ./pkg/config
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"unicode"

	"github.com/stroppy-io/stroppy/pkg/config"
)

const schemaFileMode = 0o644

var (
	logLevelType = reflect.TypeOf(config.LogLevel(0))
	logModeType  = reflect.TypeOf(config.LogMode(0))
)

func main() {
	out := flag.String("out", "", "output file path (defaults to stdout)")

	flag.Parse()

	data, err := json.MarshalIndent(buildSchema(), "", "  ")
	if err != nil {
		fatal(err)
	}

	data = append(data, '\n')

	if *out == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			fatal(err)
		}

		return
	}

	if err := os.WriteFile(*out, data, schemaFileMode); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "jsonschema-gen:", err)
	os.Exit(1)
}

func buildSchema() map[string]any {
	defs := map[string]any{}
	resolveType(reflect.TypeOf(config.RunConfig{}), defs)

	return map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$ref":        "#/$defs/RunConfig",
		"$defs":       defs,
		"title":       "Stroppy run configuration",
		"description": "The stroppy-config.json file envelope, including typed run and workload parameters.",
	}
}

func resolveType(target reflect.Type, defs map[string]any) any {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}

	if target == logLevelType {
		return enumSchema(logLevelNames(), len(config.LogLevelValues()))
	}

	if target == logModeType {
		return enumSchema(logModeNames(), len(config.LogModeValues()))
	}

	if target.Kind() == reflect.Struct {
		name := target.Name()
		if _, exists := defs[name]; !exists {
			defs[name] = structSchema(target, defs)
		}

		return map[string]any{"$ref": "#/$defs/" + name}
	}

	switch target.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int32:
		return int32Schema()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int64:
		return map[string]any{"type": "integer", "x-stroppy-bareInteger": true}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return map[string]any{"type": "integer", "minimum": 0, "x-stroppy-bareInteger": true}
	case reflect.Uint64:
		return map[string]any{
			"type":                     "integer",
			"minimum":                  0,
			"maximum":                  uint64(math.MaxUint64),
			"x-stroppy-bareInteger":    true,
			"x-stroppy-rejectExponent": true,
			"description":              "A bare unsigned JSON integer; quoted, decimal-point, and exponent forms are rejected.",
		}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{
			"type":  "array",
			"items": resolveType(target.Elem(), defs),
		}
	case reflect.Map:
		return mapSchema(target, defs)
	default:
		return map[string]any{}
	}
}

func structSchema(target reflect.Type, defs map[string]any) map[string]any {
	properties := map[string]any{}
	aliasExclusions := make([]any, 0)

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}

	for index := range target.NumField() {
		field := target.Field(index)
		if !field.IsExported() {
			continue
		}

		canonical := jsonName(&field)
		if canonical == "-" {
			continue
		}

		var property any

		switch field.Tag.Get("configscope") {
		case "run":
			property = runScopeSchema()
		case "params":
			property = paramsScopeSchema()
		default:
			property = nullable(resolveType(field.Type, defs))
		}

		properties[canonical] = property

		alias := camelToSnake(canonical)
		if alias == canonical {
			continue
		}

		properties[alias] = property
		aliasExclusions = append(aliasExclusions, aliasCollision(canonical, alias))
	}

	if len(aliasExclusions) > 0 {
		schema["allOf"] = aliasExclusions
	}

	return schema
}

func mapSchema(target reflect.Type, defs map[string]any) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": resolveType(target.Elem(), defs),
	}

	if target.Key().Kind() == reflect.Uint32 {
		schema["propertyNames"] = map[string]any{
			"pattern":             "^[0-9]+$",
			"x-stroppy-uint32Key": true,
			"description":         "Unsigned decimal uint32; leading zeroes are accepted and canonicalized.",
		}
	}

	return schema
}

func nullable(value any) map[string]any {
	return map[string]any{
		"anyOf": []any{
			value,
			map[string]any{"type": "null"},
		},
	}
}

func enumSchema(names []string, count int) map[string]any {
	ordinals := make([]int, count)
	for index := range count {
		ordinals[index] = index
	}

	return map[string]any{
		"anyOf": []any{
			map[string]any{"type": "string", "enum": names},
			map[string]any{"type": "integer", "enum": ordinals},
		},
	}
}

func int32Schema() map[string]any {
	return map[string]any{
		"anyOf": []any{
			map[string]any{
				"type":       "number",
				"minimum":    math.MinInt32,
				"maximum":    math.MaxInt32,
				"multipleOf": 1,
			},
			map[string]any{
				"type":                 "string",
				"pattern":              `^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$`,
				"x-stroppy-exactInt32": true,
				"description": "Must evaluate exactly to an in-range int32; " +
					"fractional and overflowing values are rejected.",
			},
		},
	}
}

func runScopeSchema() map[string]any {
	queryTimeout := map[string]any{
		"type":        "string",
		"description": "Per-statement query deadline as a Go duration; 0 disables it.",
	}
	properties := map[string]any{
		"executor":      map[string]any{"type": "string"},
		"vus":           bareIntegerSchema(),
		"iterations":    bareIntegerSchema(),
		"duration":      map[string]any{"type": "string"},
		"queryTimeout":  queryTimeout,
		"query_timeout": queryTimeout,
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"allOf": []any{
			aliasCollision("queryTimeout", "query_timeout"),
		},
	}
}

func bareIntegerSchema() map[string]any {
	return map[string]any{
		"type":                     "integer",
		"x-stroppy-bareInteger":    true,
		"x-stroppy-rejectExponent": true,
		"description":              "A bare JSON integer; quoted, decimal-point, and exponent forms are rejected.",
	}
}

func paramsScopeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"propertyNames": map[string]any{
			"anyOf": []any{
				map[string]any{"pattern": "^[a-z][A-Za-z0-9]*$"},
				map[string]any{"pattern": "^[a-z][a-z0-9]*(_[a-z0-9]+)+$"},
			},
		},
		"additionalProperties": map[string]any{
			"anyOf": []any{
				map[string]any{"type": "string"},
				map[string]any{"type": "boolean"},
				map[string]any{"type": "number"},
			},
		},
		"description": "Selected-workload parameter names. Snake-case aliases are canonicalized " +
			"to lower camel case; null and container values are rejected.",
		"x-stroppy-aliasCollision": "snake-to-lower-camel",
	}
}

func aliasCollision(canonical, alias string) map[string]any {
	return map[string]any{
		"not": map[string]any{
			"required": []string{canonical, alias},
		},
	}
}

func logLevelNames() []string {
	values := config.LogLevelValues()

	names := make([]string, len(values))
	for index, value := range values {
		names[index] = value.String()
	}

	return names
}

func logModeNames() []string {
	values := config.LogModeValues()

	names := make([]string, len(values))
	for index, value := range values {
		names[index] = value.String()
	}

	return names
}

func jsonName(field *reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name != "" {
		return name
	}

	runes := []rune(field.Name)
	runes[0] = unicode.ToLower(runes[0])

	return string(runes)
}

func camelToSnake(name string) string {
	var alias strings.Builder

	for index, char := range name {
		if unicode.IsUpper(char) {
			if index > 0 {
				alias.WriteByte('_')
			}

			alias.WriteRune(unicode.ToLower(char))

			continue
		}

		alias.WriteRune(char)
	}

	return alias.String()
}
