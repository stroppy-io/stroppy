// Command jsonschema-gen emits the JSON Schema for the plain-Go config package
// under pkg/config. It reflects over config.RunConfig and transitively
// referenced struct types so docs/jsonschema/run.schema.json never drifts from
// the Go contract. Regenerate with:
//
//	go generate ./pkg/config
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/stroppy-io/stroppy/pkg/config"
)

// schemaFileMode is the permission for the generated schema file. It is
// committed to the repo, so it stays world-readable like the rest of docs/.
const schemaFileMode = 0o644

// enumValues maps Go type name -> the allowed string values for the enum-backed
// string types that reflection cannot enumerate. The values are derived from the
// constant lists exported by pkg/config so the schema cannot drift from the code.
var enumValues = map[string][]string{
	"LogLevel": enumStrings(config.LogLevelValues()),
	"LogMode":  enumStrings(config.LogModeValues()),
}

func enumStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}

	return out
}

func main() {
	out := flag.String("out", "", "output file path (defaults to stdout)")

	flag.Parse()

	schema := buildSchema()

	data, err := json.MarshalIndent(schema, "", "  ")
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

	err = os.WriteFile(*out, data, schemaFileMode)
	if err != nil {
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
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$ref":    "#/$defs/RunConfig",
		"$defs":   defs,
	}
}

// resolveType returns the JSON Schema for t, registering any struct definition
// encountered into defs.
func resolveType(t reflect.Type, defs map[string]any) any {
	// Pointer fields are optional: they accept null as well as their base type.
	if t.Kind() == reflect.Pointer {
		return map[string]any{"anyOf": []any{
			resolveType(t.Elem(), defs),
			map[string]any{"type": "null"},
		}}
	}

	if t.Kind() == reflect.Struct {
		name := t.Name()
		if _, ok := defs[name]; !ok {
			defs[name] = structSchema(t, defs)
		}

		return map[string]any{"$ref": "#/$defs/" + name}
	}

	if enum := enumType(t); enum != nil {
		return enum
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": resolveType(t.Elem(), defs)}
	case reflect.Map:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": resolveType(t.Elem(), defs),
		}
	default:
		return map[string]any{}
	}
}

func enumType(t reflect.Type) map[string]any {
	if t.Kind() != reflect.String {
		return nil
	}

	values, ok := enumValues[t.Name()]
	if !ok {
		return nil
	}

	return map[string]any{"type": "string", "enum": values}
}

func structSchema(t reflect.Type, defs map[string]any) map[string]any {
	// Config structs are acyclic, so fields resolved here are registered in
	// defs by resolveType before any deeper reference to the same type.
	properties := map[string]any{}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}

	for i := range t.NumField() {
		field := t.Field(i)

		name := jsonName(&field)
		if name == "-" {
			continue
		}

		properties[name] = resolveType(field.Type, defs)
	}

	return schema
}

func jsonName(field *reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return strings.ToLower(field.Name[:1]) + field.Name[1:]
	}

	name, _, _ := strings.Cut(tag, ",")

	return name
}
