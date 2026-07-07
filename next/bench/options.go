package bench

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// durationType is the reflect.Type of time.Duration, matched specially so a
// duration field registers as a duration param (parsed with ParseDuration)
// rather than a raw int64.
var durationType = reflect.TypeOf(time.Duration(0))

// parseOptions is the struct-tag bridge onto the typed param system: it walks
// opts' exported fields and declares each `env:"NAME"`-tagged field as a typed
// param on set (resolved immediately from cli > env > config > default), then
// copies the resolved value back into the field. The param NAME is the lowercased
// env tag (WAREHOUSES -> --warehouses / config "warehouses" / env WAREHOUSES),
// so a v5 env name ports verbatim and gains uniform cli + config projections for
// free. Supported field types: string, int, int64, uint, uint64, float64, bool,
// time.Duration. An optional `help:"..."` tag supplies the --help line. A nil
// opts is a no-op.
//
// Per-field parse failures are recorded on set (not returned) so a probe can
// still surface the full catalog; the caller checks [paramSet.Err].
func parseOptions(opts any, set *paramSet) error {
	if opts == nil {
		return nil
	}
	v := reflect.ValueOf(opts)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("bench: Test.Opts must be a non-nil pointer to a struct, got %T", opts)
	}
	sv := v.Elem()
	st := sv.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		if !f.IsExported() {
			continue
		}
		env := f.Tag.Get("env")
		if env == "" || env == "-" {
			continue
		}
		name := strings.ToLower(env)
		help := f.Tag.Get("help")
		def := f.Tag.Get("default")
		fv := sv.Field(i)
		k := f.Type.Kind()
		switch {
		case f.Type == durationType:
			fv.SetInt(int64(set.Duration(name, parseDur(def), help, optEnv(env)).Value()))
		case k == reflect.String:
			fv.SetString(set.String(name, def, help, optEnv(env)).Value())
		case k == reflect.Bool:
			fv.SetBool(set.Bool(name, parseBoolDefault(def), help, optEnv(env)).Value())
		case k == reflect.Int:
			fv.SetInt(int64(set.Int(name, parseIntDefault(def), help, optEnv(env)).Value()))
		case k == reflect.Int64: // non-duration int64
			fv.SetInt(set.Int64(name, parseInt64Default(def), help, optEnv(env)).Value())
		case k == reflect.Uint || k == reflect.Uint64:
			fv.SetUint(set.Uint64(name, parseUintDefault(def), help, optEnv(env)).Value())
		case k == reflect.Float64:
			fv.SetFloat(set.Float64(name, parseFloatDefault(def), help, optEnv(env)).Value())
		default:
			return fmt.Errorf("bench: option %s: unsupported type %s", f.Name, f.Type)
		}
	}
	return nil
}

// parseDur parses a default-tag duration; an empty/unparseable tag is the zero
// duration (the field's zero value), surfaced as such rather than failing init.
func parseDur(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}
func parseBoolDefault(s string) bool    { b, _ := strconv.ParseBool(s); return b }
func parseIntDefault(s string) int       { n, _ := strconv.Atoi(s); return n }
func parseInt64Default(s string) int64   { n, _ := strconv.ParseInt(s, 10, 64); return n }
func parseUintDefault(s string) uint64   { n, _ := strconv.ParseUint(s, 10, 64); return n }
func parseFloatDefault(s string) float64 { x, _ := strconv.ParseFloat(s, 64); return x }

// validateOptions calls opts.Validate() when opts implements the hook.
func validateOptions(opts any) error {
	if v, ok := opts.(interface{ Validate() error }); ok {
		return v.Validate()
	}
	return nil
}
