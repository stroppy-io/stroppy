package bench

import (
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// OptionSchema is one option's probe description: its env variable, struct
// field, type, declared default and current (post-parse) value.
type OptionSchema struct {
	Name    string `json:"name"`    // env variable
	Field   string `json:"field"`   // struct field name
	Type    string `json:"type"`    // string|int|int64|uint64|float64|bool|duration
	Default string `json:"default"` // declared default tag
	Current string `json:"current"` // value after env/default parsing
}

// durationType is the reflect.Type of time.Duration, matched specially so a
// duration field parses with time.ParseDuration rather than as a raw int64.
var durationType = reflect.TypeOf(time.Duration(0))

// parseOptions fills opts (a pointer to a struct) from the environment via each
// exported field's `env:"NAME"` tag, falling back to its `default:"..."` tag,
// and returns the option schema for probing. Fields without an env tag (or with
// `env:"-"`) are ignored. Supported types: string, int, int64, uint64, float64,
// bool, time.Duration. A nil opts is a no-op.
func parseOptions(opts any, getenv func(string) string) ([]OptionSchema, error) {
	if opts == nil {
		return nil, nil
	}
	v := reflect.ValueOf(opts)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("bench: Test.Opts must be a non-nil pointer to a struct, got %T", opts)
	}

	sv := v.Elem()
	st := sv.Type()
	var schema []OptionSchema
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		if !f.IsExported() {
			continue
		}
		env := f.Tag.Get("env")
		if env == "" || env == "-" {
			continue
		}
		def := f.Tag.Get("default")

		raw := getenv(env)
		if raw == "" {
			raw = def
		}
		fv := sv.Field(i)
		if raw != "" {
			if err := setField(fv, raw); err != nil {
				return nil, fmt.Errorf("bench: option %s (env %s): %w", f.Name, env, err)
			}
		}
		schema = append(schema, OptionSchema{
			Name:    env,
			Field:   f.Name,
			Type:    optionType(f.Type),
			Default: def,
			Current: formatValue(fv),
		})
	}
	return schema, nil
}

// setField parses raw into fv according to fv's type.
func setField(fv reflect.Value, raw string) error {
	if fv.Type() == durationType {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		fv.SetInt(int64(d))
		return nil
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return err
		}
		fv.SetUint(n)
	case reflect.Float64:
		x, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(x)
	default:
		return fmt.Errorf("unsupported option type %s", fv.Type())
	}
	return nil
}

// optionType names t for the schema.
func optionType(t reflect.Type) string {
	if t == durationType {
		return "duration"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int64:
		return "int64"
	case reflect.Uint, reflect.Uint64:
		return "uint64"
	case reflect.Float64:
		return "float64"
	default:
		return t.Kind().String()
	}
}

// formatValue renders fv's current value for the schema.
func formatValue(fv reflect.Value) string {
	if fv.Type() == durationType {
		return time.Duration(fv.Int()).String()
	}
	return fmt.Sprintf("%v", fv.Interface())
}

// validateOptions calls opts.Validate() when opts implements the hook.
func validateOptions(opts any) error {
	if v, ok := opts.(interface{ Validate() error }); ok {
		return v.Validate()
	}
	return nil
}
