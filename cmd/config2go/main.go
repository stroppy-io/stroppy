// config2go is utility to convert config files into GO Syntax literals.
// Useful to embed configs into code in semi automatic manner.
package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	stroppy "github.com/stroppy-io/stroppy/proto/build/go/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/utils/protoyaml"
)

// TODO: add entire file generation (with package, imports, formatting)
func main() {
	var (
		configName   string
		configPath   string
		helperNeeded bool
	)

	flag.StringVar(&configPath, "i", "stroppy.json", "path to config file (json|yaml|yml)")
	flag.StringVar(&configName, "name", "Example", "name of config")
	flag.BoolVar(&helperNeeded, "helpers", false, "if helper functions generation needed")
	flag.Parse()

	_, err := os.Stat(configPath)
	if err != nil {
		fmt.Fprintf(os.Stdout, "can't find file '%s': %s\n", configPath, err)
		flag.Usage()

		return
	}

	var configBytes []byte

	configBytes, err = os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stdout, "can't read file: %s\n", err)

		return
	}

	var cfg stroppy.ConfigFile

	switch ext := path.Ext(configPath); ext {
	case ".json":
		err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(configBytes, &cfg)
		if err != nil {
			fmt.Fprintf(os.Stdout, "can't unmarshall json config: %s\n", err)

			return
		}
	case ".yaml", ".yml":
		err = protoyaml.Unmarshal(configBytes, &cfg)
		if err != nil {
			fmt.Fprintf(os.Stdout, "can't unmarshall yaml config: %s\n", err)

			return
		}
	default:
		fmt.Fprintf(os.Stdout, "unmatched extension %s", ext)
	}

	if helperNeeded {
		fmt.Fprintf(os.Stdout, "// Helper function for creating pointers with correct type"+
			"func ptr[T any](x T) *T {"+
			"	return &x"+
			"}\n\n")
	}

	fmt.Fprintf(os.Stdout, `//nolint:mnd,funlen,lll,maintidx // a huge and long magic constant
func %sConfig() *proto.ConfigFile {
	return %s
}
`, configName, ProtoToGoLiteral(&cfg))
}

// NOTE: Code below is LLM-generated and requires manual tuning later.
// TODO: refactor, fix indentations.

func ProtoToGoLiteral(msg any) string {
	return protoToLiteralRecursive(reflect.ValueOf(msg), reflect.TypeOf(msg), 0)
}

const nilStrConst = "nil"

func protoToLiteralRecursive(value reflect.Value, rType reflect.Type, depth int) string {
	// Handle pointers by dereferencing
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nilStrConst
		}

		// For primitive pointer types, use ptr() helper with correct type
		elemType := rType.Elem()
		if isPrimitiveType(elemType) {
			elemValue := protoToLiteralRecursive(value.Elem(), elemType, depth)

			return fmt.Sprintf("ptr[%s](%s)", getTypeNameWithPackage(elemType), elemValue)
		}

		// For struct pointers, add & prefix and recurse
		elemStr := protoToLiteralRecursive(value.Elem(), elemType, depth)

		return "&" + elemStr
	}

	// Handle slices
	if value.Kind() == reflect.Slice {
		return handleSlice(value, rType, depth)
	}

	// Handle maps
	if value.Kind() == reflect.Map {
		return handleMap(value, rType, depth)
	}

	// Handle structs
	if value.Kind() == reflect.Struct {
		return handleStruct(value, rType, depth)
	}

	// Handle interface{} and any other complex types that might contain pointers
	if value.Kind() == reflect.Interface && !value.IsNil() {
		actualValue := value.Elem()
		actualType := actualValue.Type()

		return protoToLiteralRecursive(actualValue, actualType, depth)
	}

	// Handle primitives with proper formatting
	return formatPrimitiveValue(value, rType)
}

func formatPrimitiveValue(value reflect.Value, rType reflect.Type) string {
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Handle enum types
		if isEnumType(rType) {
			return fmt.Sprintf("%s(%d)", getTypeNameWithPackage(rType), value.Int())
		}

		return strconv.FormatInt(value.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		// Handle enum types
		if isEnumType(rType) {
			return fmt.Sprintf("%s(%d)", getTypeNameWithPackage(rType), value.Uint())
		}

		return strconv.FormatUint(value.Uint(), 10)
	case reflect.Float32:
		return fmt.Sprintf("%g", value.Float())
	case reflect.Float64:
		return fmt.Sprintf("%g", value.Float())
	case reflect.String:
		return fmt.Sprintf("%q", value.String())
	case reflect.Bool:
		return strconv.FormatBool(value.Bool())
	case reflect.Uintptr:
		// This handles cases where we get memory addresses - we should avoid these
		return "nil  // ERROR: Got memory address instead of proper value"
	default:
		// Check if it's a complex value that wasn't handled properly
		valueStr := fmt.Sprintf("%#v", value.Interface())
		if strings.Contains(valueStr, "0x") && strings.Contains(valueStr, "(") {
			return "nil  // ERROR: Memory address detected, value not properly dereferenced"
		}

		return valueStr
	}
}

func handleSlice(value reflect.Value, rType reflect.Type, depth int) string {
	if value.Len() == 0 {
		return nilStrConst
	}

	var elements []string

	elemType := rType.Elem()

	// Check if it's a slice of pointers
	if elemType.Kind() == reflect.Pointer {
		for i := range value.Len() {
			elem := value.Index(i)
			if elem.IsNil() {
				elements = append(elements, nilStrConst)
			} else {
				// For pointer elements, we need to dereference and add & prefix
				elemStr := protoToLiteralRecursive(elem, elemType, depth)
				elements = append(elements, elemStr)
			}
		}
	} else {
		// Regular slice (not pointers)
		for i := range value.Len() {
			elem := protoToLiteralRecursive(value.Index(i), elemType, depth)
			elements = append(elements, elem)
		}
	}

	// Format the slice literal with package name
	sliceTypeName := getTypeNameWithPackage(rType)

	const oneLineSliceElementsCount = 3

	if len(elements) <= oneLineSliceElementsCount {
		return fmt.Sprintf("%s{%s}", sliceTypeName, strings.Join(elements, ", "))
	}
	// Multi-line for longer slices
	indent := strings.Repeat("    ", depth+1)
	nextIndent := strings.Repeat("    ", depth+1)
	formattedElements := make([]string, len(elements))

	for i, elem := range elements {
		formattedElements[i] = nextIndent + elem
	}

	return fmt.Sprintf(
		"%s{\n%s,\n%s}",
		sliceTypeName,
		strings.Join(formattedElements, ",\n"),
		indent,
	)
}

func handleMap(v reflect.Value, rType reflect.Type, depth int) string {
	if v.Len() == 0 {
		return nilStrConst
	}

	pairs := make([]string, 0, v.Len())

	for _, key := range v.MapKeys() {
		keyStr := formatPrimitiveValue(key, key.Type())
		valueStr := protoToLiteralRecursive(v.MapIndex(key), rType.Elem(), depth)
		pairs = append(pairs, fmt.Sprintf("%s: %s", keyStr, valueStr))
	}

	mapTypeName := getTypeNameWithPackage(rType)

	return fmt.Sprintf("%s{%s}", mapTypeName, strings.Join(pairs, ", "))
}

func handleStruct(v reflect.Value, t reflect.Type, depth int) string {
	typeName := getTypeNameWithPackage(t)

	var fields []string

	indent := strings.Repeat("    ", depth)
	nextIndent := strings.Repeat("    ", depth+1)

	for i := range v.NumField() {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Skip protobuf internal fields
		if isInternalProtoField(field.Name) {
			continue
		}

		// Skip zero values for cleaner output, but be careful with pointers
		if isZeroValue(fieldValue) {
			continue
		}

		// Special handling for interface fields (common in oneof)
		if field.Type.Kind() == reflect.Interface {
			if fieldValue.IsNil() {
				continue
			}
			// Get the actual type and value from the interface
			actualValue := fieldValue.Elem()
			actualType := actualValue.Type()

			fieldLiteral := protoToLiteralRecursive(actualValue, actualType, depth+1)
			fields = append(fields, fmt.Sprintf("%s%s: %s", nextIndent, field.Name, fieldLiteral))
		} else {
			fieldLiteral := protoToLiteralRecursive(fieldValue, field.Type, depth+1)
			fields = append(fields, fmt.Sprintf("%s%s: %s", nextIndent, field.Name, fieldLiteral))
		}
	}

	if len(fields) == 0 {
		return typeName + "{}"
	}

	return fmt.Sprintf("%s{\n%s,\n%s}", typeName, strings.Join(fields, ",\n"), indent)
}

func getTypeNameWithPackage(rType reflect.Type) string {
	// Get the base type name
	typeName := rType.Name()
	if typeName != "" {
		// Get package path
		pkgPath := rType.PkgPath()
		if pkgPath == "" {
			// Built-in types
			return typeName
		}

		// Extract package name (last part of the path)
		pkgName := pkgPath
		if lastSlash := strings.LastIndex(pkgPath, "/"); lastSlash != -1 {
			pkgName = pkgPath[lastSlash+1:]
		}

		return fmt.Sprintf("%s.%s", pkgName, typeName)
	}

	// For types like slices, maps, pointers, use the full string representation
	fullName := rType.String()
	// Clean up built-in types
	if strings.Contains(fullName, "uint64") {
		return "uint64"
	}

	if strings.Contains(fullName, "uint32") {
		return "uint32"
	}

	if strings.Contains(fullName, "int64") {
		return "int64"
	}

	if strings.Contains(fullName, "int32") {
		return "int32"
	}

	if strings.Contains(fullName, "int") && !strings.Contains(fullName, "int64") &&
		!strings.Contains(fullName, "int32") {
		return "int"
	}

	return fullName
}

func isPrimitiveType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	default:
		return false
	}
}

func isEnumType(t reflect.Type) bool {
	// Check if it's likely an enum (named type based on int/uint)
	if t.PkgPath() == "" {
		return false
	}

	switch t.Kind() {
	case reflect.Int32, reflect.Uint32, reflect.Int, reflect.Uint:
		return t.Name() != "" && t.Name() != "int32" && t.Name() != "uint32" && t.Name() != "int" &&
			t.Name() != "uint"

	default:
		return false
	}
}

func isInternalProtoField(name string) bool {
	return strings.HasPrefix(name, "XXX_") ||
		strings.HasPrefix(name, "state") ||
		strings.HasPrefix(name, "sizeCache") ||
		strings.HasPrefix(name, "unknownFields") ||
		name == "UnknownFields"
}

func isZeroValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.String:
		return value.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return value.Float() == 0.0
	case reflect.Bool:
		return !value.Bool()
	case reflect.Slice, reflect.Array:
		return value.Len() == 0
	case reflect.Map:
		return value.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return value.IsNil()
	default:
		return false
	}
}
