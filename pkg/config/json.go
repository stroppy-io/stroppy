package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	aliasesPerField       = 2
	decimalRadix          = 10
	maxInt32DecimalDigits = 10
)

var (
	logLevelType = reflect.TypeOf(LogLevel(0))
	logModeType  = reflect.TypeOf(LogMode(0))

	jsonIntegerPattern  = regexp.MustCompile(`^-?(?:0|[1-9]\d*)$`)
	protoIntegerPattern = regexp.MustCompile(`^(-?)(0|[1-9]\d*)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$`)
	lowerCamelPattern   = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	snakePattern        = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)+$`)
	uintMapKeyPattern   = regexp.MustCompile(`^\d+$`)
)

var (
	errInvalidUnmarshalTarget = errors.New("config unmarshal target must be a non-nil pointer")
	errInvalidConfigJSON      = errors.New("invalid config JSON")
	errInvalidInt32           = errors.New("invalid int32")
)

// Unmarshal validates and canonicalizes the complete JSON token stream before
// decoding it into a plain config type. It preserves the former ProtoJSON field
// aliases and scalar forms while rejecting duplicate, colliding, mis-cased, and
// unknown fields at every nesting level.
func Unmarshal(data []byte, dst any) error {
	target := reflect.ValueOf(dst)
	if !target.IsValid() || target.Kind() != reflect.Pointer || target.IsNil() {
		return errInvalidUnmarshalTarget
	}

	normalized, err := normalizeDocument(data, target.Type().Elem())
	if err != nil {
		return err
	}

	target.Elem().Set(reflect.Zero(target.Type().Elem()))

	if err := json.Unmarshal(normalized, dst); err != nil {
		return fmt.Errorf("decode normalized config: %w", err)
	}

	return nil
}

func normalizeDocument(data []byte, target reflect.Type) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var out bytes.Buffer
	if err := normalizeValue(decoder, target, "$", false, "", &out); err != nil {
		return nil, err
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("$: %w: trailing JSON data", errInvalidConfigJSON)
	}

	return out.Bytes(), nil
}

type structField struct {
	canonical string
	typ       reflect.Type
	scope     string
}

func normalizeValue(
	decoder *json.Decoder,
	target reflect.Type,
	path string,
	allowNull bool,
	scope string,
	out *bytes.Buffer,
) error {
	if scope != "" {
		return normalizeScope(decoder, scope, path, out)
	}

	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}

	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if token == nil {
		if !allowNull {
			return fmt.Errorf("%s: %w: null is not allowed", path, errInvalidConfigJSON)
		}

		out.WriteString("null")

		return nil
	}

	if target == logLevelType {
		return normalizeEnum(token, path, logLevelNames, out)
	}

	if target == logModeType {
		return normalizeEnum(token, path, logModeNames, out)
	}

	switch target.Kind() {
	case reflect.Struct:
		return normalizeStruct(decoder, token, target, path, out)
	case reflect.Map:
		return normalizeMap(decoder, token, target, path, out)
	case reflect.Slice:
		return normalizeSlice(decoder, token, target.Elem(), path, out)
	default:
		return normalizeScalar(token, target, path, out)
	}
}

func normalizeScalar(token json.Token, target reflect.Type, path string, out *bytes.Buffer) error {
	switch target.Kind() {
	case reflect.String:
		value, ok := token.(string)
		if !ok {
			return typeError(path, "string", token)
		}

		writeJSONString(out, value)

		return nil
	case reflect.Bool:
		value, ok := token.(bool)
		if !ok {
			return typeError(path, "boolean", token)
		}

		out.WriteString(strconv.FormatBool(value))

		return nil
	case reflect.Int32:
		value, err := normalizeProtoInt32(token)
		if err != nil {
			return fmt.Errorf(
				"%s: %w: invalid int32 value %s",
				path,
				errInvalidConfigJSON,
				displayToken(token),
			)
		}

		out.WriteString(value)

		return nil
	case reflect.Uint64:
		return normalizeSeed(token, path, out)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int64:
		return normalizeSignedJSONInteger(token, target.Bits(), path, out)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return normalizeUnsignedJSONInteger(token, target.Bits(), path, out)
	default:
		return fmt.Errorf("%s: %w: unsupported config type %s", path, errInvalidConfigJSON, target)
	}
}

func normalizeSeed(token json.Token, path string, out *bytes.Buffer) error {
	number, ok := token.(json.Number)
	if !ok || !jsonIntegerPattern.MatchString(number.String()) || strings.HasPrefix(number.String(), "-") {
		return fmt.Errorf("%s: %w: expected a bare unsigned JSON integer", path, errInvalidConfigJSON)
	}

	value, err := strconv.ParseUint(number.String(), decimalRadix, 64)
	if err != nil {
		return fmt.Errorf("%s: %w: unsigned integer out of range", path, errInvalidConfigJSON)
	}

	out.WriteString(strconv.FormatUint(value, decimalRadix))

	return nil
}

func normalizeStruct(
	decoder *json.Decoder,
	token json.Token,
	target reflect.Type,
	path string,
	out *bytes.Buffer,
) error {
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return typeError(path, "object", token)
	}

	fields := structFields(target)
	seen := make(map[string]string)
	first := true

	out.WriteByte('{')

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		key, ok := keyToken.(string)
		if !ok {
			return typeError(path, "object field name", keyToken)
		}

		field, ok := fields[key]
		if !ok {
			return fmt.Errorf("%s: %w: unknown field %q", fieldPath(path, key), errInvalidConfigJSON, key)
		}

		valuePath := fieldPath(path, field.canonical)
		if previous, duplicate := seen[field.canonical]; duplicate {
			return duplicateFieldError(valuePath, previous, key)
		}

		seen[field.canonical] = key

		if !first {
			out.WriteByte(',')
		}

		first = false

		writeJSONString(out, field.canonical)
		out.WriteByte(':')

		if err := normalizeValue(decoder, field.typ, valuePath, true, field.scope, out); err != nil {
			return err
		}
	}

	closing, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return typeError(path, "object", closing)
	}

	out.WriteByte('}')

	return nil
}

func structFields(target reflect.Type) map[string]structField {
	fields := make(map[string]structField, target.NumField()*aliasesPerField)

	for index := range target.NumField() {
		field := target.Field(index)
		if !field.IsExported() {
			continue
		}

		canonical := jsonFieldName(&field)
		if canonical == "-" {
			continue
		}

		entry := structField{
			canonical: canonical,
			typ:       field.Type,
			scope:     field.Tag.Get("configscope"),
		}
		fields[canonical] = entry

		alias := camelToSnake(canonical)
		if alias != canonical {
			fields[alias] = entry
		}

		for _, alias = range strings.Split(field.Tag.Get("configaliases"), ",") {
			if alias != "" {
				fields[alias] = entry
			}
		}
	}

	return fields
}

func jsonFieldName(field *reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name != "" {
		return name
	}

	runes := []rune(field.Name)
	runes[0] = unicode.ToLower(runes[0])

	return string(runes)
}

func normalizeMap(
	decoder *json.Decoder,
	token json.Token,
	target reflect.Type,
	path string,
	out *bytes.Buffer,
) error {
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return typeError(path, "object", token)
	}

	seen := make(map[string]string)
	first := true

	out.WriteByte('{')

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		key, ok := keyToken.(string)
		if !ok {
			return typeError(path, "map key", keyToken)
		}

		canonical, err := normalizeMapKey(key, target.Key())
		if err != nil {
			return fmt.Errorf("%s: %w", mapPath(path, key), err)
		}

		entryPath := mapPath(path, canonical)
		if previous, duplicate := seen[canonical]; duplicate {
			return duplicateMapKeyError(entryPath, previous, key)
		}

		seen[canonical] = key

		if !first {
			out.WriteByte(',')
		}

		first = false

		writeJSONString(out, canonical)
		out.WriteByte(':')

		if err := normalizeValue(decoder, target.Elem(), entryPath, false, "", out); err != nil {
			return err
		}
	}

	closing, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return typeError(path, "object", closing)
	}

	out.WriteByte('}')

	return nil
}

func normalizeMapKey(key string, target reflect.Type) (string, error) {
	switch target.Kind() {
	case reflect.String:
		return key, nil
	case reflect.Uint32:
		if !uintMapKeyPattern.MatchString(key) {
			return "", fmt.Errorf("%w: invalid uint32 map key %q", errInvalidConfigJSON, key)
		}

		value, err := strconv.ParseUint(key, decimalRadix, 32)
		if err != nil {
			return "", fmt.Errorf("%w: invalid uint32 map key %q", errInvalidConfigJSON, key)
		}

		return strconv.FormatUint(value, decimalRadix), nil
	default:
		return "", fmt.Errorf("%w: unsupported map key type %s", errInvalidConfigJSON, target)
	}
}

func normalizeSlice(
	decoder *json.Decoder,
	token json.Token,
	element reflect.Type,
	path string,
	out *bytes.Buffer,
) error {
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return typeError(path, "array", token)
	}

	index := 0

	out.WriteByte('[')

	for decoder.More() {
		if index > 0 {
			out.WriteByte(',')
		}

		if err := normalizeValue(decoder, element, indexPath(path, index), false, "", out); err != nil {
			return err
		}

		index++
	}

	closing, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if delimiter, ok := closing.(json.Delim); !ok || delimiter != ']' {
		return typeError(path, "array", closing)
	}

	out.WriteByte(']')

	return nil
}

type scopeScalar int

const (
	scopeAnyScalar scopeScalar = iota
	scopeString
	scopeInteger
)

var runScopeFields = map[string]scopeScalar{
	"executor":     scopeString,
	"vus":          scopeInteger,
	"iterations":   scopeInteger,
	"duration":     scopeString,
	"queryTimeout": scopeString,
}

func normalizeScope(decoder *json.Decoder, scope, path string, out *bytes.Buffer) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return typeError(path, "object", token)
	}

	seen := make(map[string]string)
	first := true

	out.WriteByte('{')

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		key, ok := keyToken.(string)
		if !ok {
			return typeError(path, "object field name", keyToken)
		}

		canonical, valueKind, err := scopeField(scope, path, key)
		if err != nil {
			return err
		}

		valuePath := fieldPath(path, canonical)
		if previous, duplicate := seen[canonical]; duplicate {
			return duplicateFieldError(valuePath, previous, key)
		}

		seen[canonical] = key

		if !first {
			out.WriteByte(',')
		}

		first = false

		writeJSONString(out, canonical)
		out.WriteByte(':')

		if err := normalizeScopeScalar(decoder, valueKind, valuePath, out); err != nil {
			return err
		}
	}

	closing, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return typeError(path, "object", closing)
	}

	out.WriteByte('}')

	return nil
}

func scopeField(scope, path, key string) (string, scopeScalar, error) {
	canonical, ok := canonicalScopeName(key)
	if !ok {
		return "", scopeAnyScalar, fmt.Errorf(
			"%s: %w: invalid config field name %q",
			fieldPath(path, key),
			errInvalidConfigJSON,
			key,
		)
	}

	kind := scopeAnyScalar

	if scope == "run" {
		if knownKind, known := runScopeFields[canonical]; known {
			kind = knownKind
		}
	}

	return canonical, kind, nil
}

func canonicalScopeName(name string) (string, bool) {
	if lowerCamelPattern.MatchString(name) {
		return name, true
	}

	if !snakePattern.MatchString(name) {
		return "", false
	}

	parts := strings.Split(name, "_")

	var canonical strings.Builder
	canonical.WriteString(parts[0])

	for _, part := range parts[1:] {
		canonical.WriteString(strings.ToUpper(part[:1]))
		canonical.WriteString(part[1:])
	}

	return canonical.String(), true
}

func normalizeScopeScalar(
	decoder *json.Decoder,
	kind scopeScalar,
	path string,
	out *bytes.Buffer,
) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	switch kind {
	case scopeString:
		value, ok := token.(string)
		if !ok {
			return typeError(path, "string", token)
		}

		writeJSONString(out, value)
	case scopeInteger:
		number, ok := token.(json.Number)
		if !ok || !jsonIntegerPattern.MatchString(number.String()) {
			return typeError(path, "integer", token)
		}

		if _, err := strconv.ParseInt(number.String(), decimalRadix, 64); err != nil {
			return fmt.Errorf("%s: %w: integer out of range", path, errInvalidConfigJSON)
		}

		out.WriteString(number.String())
	case scopeAnyScalar:
		switch value := token.(type) {
		case string:
			writeJSONString(out, value)
		case bool:
			out.WriteString(strconv.FormatBool(value))
		case json.Number:
			out.WriteString(value.String())
		default:
			return fmt.Errorf("%s: %w: expected a non-null JSON scalar", path, errInvalidConfigJSON)
		}
	default:
		return fmt.Errorf("%s: %w: unsupported parameter scope value", path, errInvalidConfigJSON)
	}

	return nil
}

func normalizeEnum[T ~int32](token json.Token, path string, names map[T]string, out *bytes.Buffer) error {
	switch value := token.(type) {
	case string:
		for _, name := range names {
			if value == name {
				writeJSONString(out, value)

				return nil
			}
		}
	case json.Number:
		ordinalText, err := normalizeProtoInt32(value)
		if err == nil {
			ordinal, err := strconv.ParseInt(ordinalText, decimalRadix, 32)
			if err == nil {
				if _, ok := names[T(ordinal)]; ok {
					out.WriteString(strconv.FormatInt(ordinal, decimalRadix))

					return nil
				}
			}
		}
	}

	return fmt.Errorf(
		"%s: %w: invalid enum value %s",
		path,
		errInvalidConfigJSON,
		displayToken(token),
	)
}

func normalizeProtoInt32(token json.Token) (string, error) {
	number, ok := protoNumberText(token)
	if !ok {
		return "", errInvalidInt32
	}

	matches := protoIntegerPattern.FindStringSubmatch(number)
	if matches == nil {
		return "", errInvalidInt32
	}

	digits := strings.TrimLeft(matches[2]+matches[3], "0")
	if digits == "" {
		return "0", nil
	}

	scale, err := protoDecimalScale(matches[3], matches[4])
	if err != nil {
		return "", err
	}

	digits, err = exactIntegerDigits(digits, scale)
	if err != nil {
		return "", err
	}

	if matches[1] == "-" {
		digits = "-" + digits
	}

	value, err := strconv.ParseInt(digits, decimalRadix, 64)
	if err != nil || value < math.MinInt32 || value > math.MaxInt32 {
		return "", errInvalidInt32
	}

	return strconv.FormatInt(value, decimalRadix), nil
}

func protoNumberText(token json.Token) (string, bool) {
	switch value := token.(type) {
	case json.Number:
		return value.String(), true
	case string:
		return value, true
	default:
		return "", false
	}
}

func protoDecimalScale(fraction, exponentText string) (int64, error) {
	exponent := int64(0)

	if exponentText != "" {
		parsed, err := strconv.ParseInt(exponentText, decimalRadix, 64)
		if err != nil {
			return 0, errInvalidInt32
		}

		exponent = parsed
	}

	fractionDigits := int64(len(fraction))
	if exponent < 0 && fractionDigits > math.MaxInt64+exponent {
		return 0, errInvalidInt32
	}

	return fractionDigits - exponent, nil
}

func exactIntegerDigits(digits string, scale int64) (string, error) {
	if scale > 0 {
		if scale > int64(len(digits)) {
			return "", errInvalidInt32
		}

		cut := len(digits) - int(scale)
		if strings.TrimRight(digits[cut:], "0") != "" {
			return "", errInvalidInt32
		}

		digits = strings.TrimLeft(digits[:cut], "0")
	}

	if scale < 0 {
		zeroes := -scale
		if len(digits) > maxInt32DecimalDigits || zeroes > int64(maxInt32DecimalDigits-len(digits)) {
			return "", errInvalidInt32
		}

		digits += strings.Repeat("0", int(zeroes))
	}

	if digits == "" {
		return "0", nil
	}

	if len(digits) > maxInt32DecimalDigits {
		return "", errInvalidInt32
	}

	return digits, nil
}

func normalizeSignedJSONInteger(token json.Token, bits int, path string, out *bytes.Buffer) error {
	number, ok := token.(json.Number)
	if !ok || !jsonIntegerPattern.MatchString(number.String()) {
		return typeError(path, "integer", token)
	}

	value, err := strconv.ParseInt(number.String(), decimalRadix, bits)
	if err != nil {
		return fmt.Errorf("%s: %w: integer out of range", path, errInvalidConfigJSON)
	}

	out.WriteString(strconv.FormatInt(value, decimalRadix))

	return nil
}

func normalizeUnsignedJSONInteger(token json.Token, bits int, path string, out *bytes.Buffer) error {
	number, ok := token.(json.Number)
	if !ok || !jsonIntegerPattern.MatchString(number.String()) || strings.HasPrefix(number.String(), "-") {
		return typeError(path, "unsigned integer", token)
	}

	value, err := strconv.ParseUint(number.String(), decimalRadix, bits)
	if err != nil {
		return fmt.Errorf("%s: %w: unsigned integer out of range", path, errInvalidConfigJSON)
	}

	out.WriteString(strconv.FormatUint(value, decimalRadix))

	return nil
}

func camelToSnake(name string) string {
	var alias strings.Builder

	for index, r := range name {
		if unicode.IsUpper(r) {
			if index > 0 {
				alias.WriteByte('_')
			}

			alias.WriteRune(unicode.ToLower(r))

			continue
		}

		alias.WriteRune(r)
	}

	return alias.String()
}

func duplicateFieldError(path, first, second string) error {
	if first == second {
		return fmt.Errorf("%s: %w: duplicate field %q", path, errInvalidConfigJSON, first)
	}

	names := []string{first, second}
	sort.Strings(names)

	return fmt.Errorf(
		"%s: %w: field aliases %q and %q cannot both be set",
		path,
		errInvalidConfigJSON,
		names[0],
		names[1],
	)
}

func duplicateMapKeyError(path, first, second string) error {
	if first == second {
		return fmt.Errorf("%s: %w: duplicate map key %q", path, errInvalidConfigJSON, first)
	}

	names := []string{first, second}
	sort.Strings(names)

	return fmt.Errorf(
		"%s: %w: map keys %q and %q resolve to the same key",
		path,
		errInvalidConfigJSON,
		names[0],
		names[1],
	)
}

func typeError(path, expected string, token json.Token) error {
	return fmt.Errorf(
		"%s: %w: expected %s, got %s",
		path,
		errInvalidConfigJSON,
		expected,
		displayToken(token),
	)
}

func displayToken(token json.Token) string {
	if token == nil {
		return "null"
	}

	if text, ok := token.(string); ok {
		return strconv.Quote(text)
	}

	return fmt.Sprint(token)
}

func writeJSONString(out *bytes.Buffer, value string) {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}

	out.Write(encoded)
}

func fieldPath(path, field string) string {
	if lowerCamelPattern.MatchString(field) {
		return path + "." + field
	}

	return mapPath(path, field)
}

func mapPath(path, key string) string {
	return path + "[" + strconv.Quote(key) + "]"
}

func indexPath(path string, index int) string {
	return path + "[" + strconv.Itoa(index) + "]"
}
