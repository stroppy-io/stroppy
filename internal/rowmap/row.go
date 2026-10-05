// Package rowmap shares shallow SQL-row conversion with ordinary struct sources.
package rowmap

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func Columns(t reflect.Type) (names []string, indexes []int, err error) {
	if t.Kind() != reflect.Struct || t == reflect.TypeFor[time.Time]() {
		return nil, nil, conversionError("%s is not a row struct", t)
	}

	seen := map[string]bool{}

	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("db")
		if name == "-" {
			continue
		}

		if name == "" {
			name = strings.ToLower(field.Name)
		}

		if seen[name] {
			return nil, nil, conversionError("duplicate column %q", name)
		}

		seen[name] = true
		names = append(names, name)
		indexes = append(indexes, i)
	}

	if len(names) == 0 {
		return nil, nil, conversionError("row struct has no columns")
	}

	return names, indexes, nil
}

func Decode[T any](columns []string, values []any) (T, error) {
	var out T

	target := reflect.ValueOf(&out).Elem()
	if target.Kind() != reflect.Struct || target.Type() == reflect.TypeFor[time.Time]() {
		if len(values) != 1 {
			return out, conversionError("scalar result requires one column, got %d", len(values))
		}

		err := Assign(target, values[0])

		return out, err
	}

	names, indexes, err := Columns(target.Type())
	if err != nil {
		return out, err
	}

	positions := map[string]int{}

	for i, name := range columns {
		key := strings.ToLower(name)
		if _, exists := positions[key]; exists {
			return out, conversionError("ambiguous result column %q", name)
		}

		positions[key] = i
	}

	for i, name := range names {
		position, ok := positions[strings.ToLower(name)]
		if !ok || position >= len(values) {
			return out, conversionError("missing result column %q", name)
		}

		if err := Assign(target.Field(indexes[i]), values[position]); err != nil {
			return out, fmt.Errorf("column %q: %w", name, err)
		}
	}

	return out, nil
}

//nolint:gocognit,cyclop,funlen // nullable and scalar destination conversion stays explicit.
func Assign(dst reflect.Value, value any) error {
	if value == nil {
		if dst.Kind() == reflect.Pointer || dst.Kind() == reflect.Interface || dst.Kind() == reflect.Slice {
			dst.SetZero()

			return nil
		}

		return conversionError("NULL cannot be assigned to %s", dst.Type())
	}

	if dst.Kind() == reflect.Pointer {
		dst.Set(reflect.New(dst.Type().Elem()))

		return Assign(dst.Elem(), value)
	}

	if dst.Kind() == reflect.Interface {
		copied := reflect.ValueOf(Copy(value))
		if !copied.Type().AssignableTo(dst.Type()) {
			return conversionError("cannot assign %T to %s", value, dst.Type())
		}

		dst.Set(copied)

		return nil
	}

	if dst.Type() == reflect.TypeFor[time.Time]() {
		v, ok := value.(time.Time)
		if !ok {
			return conversionError("cannot convert %T to time.Time", value)
		}

		dst.Set(reflect.ValueOf(v))

		return nil
	}

	text := fmt.Sprint(value)
	if v, ok := value.([]byte); ok {
		text = string(v)
	}

	switch dst.Kind() {
	case reflect.String:
		dst.SetString(text)

		return nil
	case reflect.Slice:
		if dst.Type().Elem().Kind() != reflect.Uint8 {
			return conversionError("unsupported destination %s", dst.Type())
		}

		dst.SetBytes([]byte(text))

		return nil
	case reflect.Bool:
		v, err := strconv.ParseBool(text)
		if err != nil {
			return err
		}

		dst.SetBool(v)

		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(text, 10, dst.Type().Bits())
		if err != nil {
			return err
		}

		dst.SetInt(v)

		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(text, 10, dst.Type().Bits())
		if err != nil {
			return err
		}

		dst.SetUint(v)

		return nil
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(text, dst.Type().Bits())
		if err != nil {
			return err
		}

		dst.SetFloat(v)

		return nil
	default:
		return conversionError("unsupported destination %s", dst.Type())
	}
}

func Copy(value any) any {
	if v, ok := value.([]byte); ok {
		return append([]byte(nil), v...)
	}

	return value
}

var errConversion = errors.New("row conversion")

func conversionError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errConversion, fmt.Sprintf(format, args...))
}
