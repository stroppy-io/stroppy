// config2go is utility to convert config files into GO Syntax literals.
// Useful to embed configs into code in semi automatic manner.
package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/stroppy-io/stroppy/internal/protoyaml"
	"github.com/stroppy-io/stroppy/pkg/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

// TODO: add entire file generation (with package, imports, formatting)
func main() {
	var (
		configName   string
		configPath   string
		helperNeeded bool
	)
	{
		flag.StringVar(&configPath, "i", "stroppy.json", "path to config file (json|yaml|yml)")
		flag.StringVar(&configName, "name", "Example", "name of config")
		flag.BoolVar(&helperNeeded, "helpers", false, "if helper functions generation needed")
		flag.Parse()
	}
	_, err := os.Stat(configPath)
	if err != nil {
		fmt.Printf("can't find file '%s': %s\n", configPath, err)
		flag.Usage()
		return
	}
	var configBytes []byte
	configBytes, err = os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("can't read file: %s\n", err)
		return
	}
	var cfg proto.Config
	switch path.Ext(configPath) {
	case "json":
		err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(configBytes, &cfg)
		if err != nil {
			fmt.Printf("can't unmarshall json config: %s\n", err)
			return
		}
	case "yaml":
		fallthrough
	case "yml":
		err = protoyaml.Unmarshal(configBytes, &cfg)
		if err != nil {
			fmt.Printf("can't unmarshall yaml config: %s\n", err)
			return
		}
	}
	if helperNeeded {
		fmt.Printf("// Helper function for creating pointers with correct type" +
			"func ptr[T any](x T) *T {" +
			"	return &x" +
			"}\n\n")
	}
	fmt.Printf(`//nolint:mnd
func %sConfig() *proto.Config {
	return %s
}
`, configName, ProtoToGoLiteral(&cfg))
}

// NOTE: everything below is LLM generated
// TODO: refactor, fix indentations
func ProtoToGoLiteral(msg any) string {
	return protoToLiteralRecursive(reflect.ValueOf(msg), reflect.TypeOf(msg), 0)
}

func protoToLiteralRecursive(v reflect.Value, t reflect.Type, depth int) string {
	// Handle pointers by dereferencing
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "nil"
		}

		// For primitive pointer types, use ptr() helper with correct type
		elemType := t.Elem()
		if isPrimitiveType(elemType) {
			elemValue := protoToLiteralRecursive(v.Elem(), elemType, depth)
			return fmt.Sprintf("ptr[%s](%s)", getTypeNameWithPackage(elemType), elemValue)
		}

		// For struct pointers, add & prefix and recurse
		elemStr := protoToLiteralRecursive(v.Elem(), elemType, depth)
		return fmt.Sprintf("&%s", elemStr)
	}

	// Handle slices
	if v.Kind() == reflect.Slice {
		return handleSlice(v, t, depth)
	}

	// Handle maps
	if v.Kind() == reflect.Map {
		return handleMap(v, t, depth)
	}

	// Handle structs
	if v.Kind() == reflect.Struct {
		return handleStruct(v, t, depth)
	}

	// Handle interface{} and any other complex types that might contain pointers
	if v.Kind() == reflect.Interface && !v.IsNil() {
		actualValue := v.Elem()
		actualType := actualValue.Type()
		return protoToLiteralRecursive(actualValue, actualType, depth)
	}

	// Handle primitives with proper formatting
	return formatPrimitiveValue(v, t)
}

func formatPrimitiveValue(v reflect.Value, t reflect.Type) string {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Handle enum types
		if isEnumType(t) {
			return fmt.Sprintf("%s(%d)", getTypeNameWithPackage(t), v.Int())
		}
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		// Handle enum types
		if isEnumType(t) {
			return fmt.Sprintf("%s(%d)", getTypeNameWithPackage(t), v.Uint())
		}
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32:
		return fmt.Sprintf("%g", v.Float())
	case reflect.Float64:
		return fmt.Sprintf("%g", v.Float())
	case reflect.String:
		return fmt.Sprintf("%q", v.String())
	case reflect.Bool:
		return fmt.Sprintf("%t", v.Bool())
	case reflect.Uintptr:
		// This handles cases where we get memory addresses - we should avoid these
		return "nil  // ERROR: Got memory address instead of proper value"
	default:
		// Check if it's a complex value that wasn't handled properly
		valueStr := fmt.Sprintf("%#v", v.Interface())
		if strings.Contains(valueStr, "0x") && strings.Contains(valueStr, "(") {
			return "nil  // ERROR: Memory address detected, value not properly dereferenced"
		}
		return valueStr
	}
}

func handleSlice(v reflect.Value, t reflect.Type, depth int) string {
	if v.Len() == 0 {
		return "nil"
	}

	var elements []string
	elemType := t.Elem()

	// Check if it's a slice of pointers
	if elemType.Kind() == reflect.Pointer {
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i)
			if elem.IsNil() {
				elements = append(elements, "nil")
			} else {
				// For pointer elements, we need to dereference and add & prefix
				elemStr := protoToLiteralRecursive(elem, elemType, depth)
				elements = append(elements, elemStr)
			}
		}
	} else {
		// Regular slice (not pointers)
		for i := 0; i < v.Len(); i++ {
			elem := protoToLiteralRecursive(v.Index(i), elemType, depth)
			elements = append(elements, elem)
		}
	}

	// Format the slice literal with package name
	sliceTypeName := getTypeNameWithPackage(t)
	if len(elements) <= 3 {
		return fmt.Sprintf("%s{%s}", sliceTypeName, strings.Join(elements, ", "))
	} else {
		// Multi-line for longer slices
		indent := strings.Repeat("    ", depth+1)
		nextIndent := strings.Repeat("    ", depth+2)
		formattedElements := make([]string, len(elements))
		for i, elem := range elements {
			formattedElements[i] = nextIndent + elem
		}
		return fmt.Sprintf("%s{\n%s,\n%s}", sliceTypeName, strings.Join(formattedElements, ",\n"), indent)
	}
}

func handleMap(v reflect.Value, t reflect.Type, depth int) string {
	if v.Len() == 0 {
		return "nil"
	}

	var pairs []string
	for _, key := range v.MapKeys() {
		keyStr := formatPrimitiveValue(key, key.Type())
		valueStr := protoToLiteralRecursive(v.MapIndex(key), t.Elem(), depth)
		pairs = append(pairs, fmt.Sprintf("%s: %s", keyStr, valueStr))
	}

	mapTypeName := getTypeNameWithPackage(t)
	return fmt.Sprintf("%s{%s}", mapTypeName, strings.Join(pairs, ", "))
}

func handleStruct(v reflect.Value, t reflect.Type, depth int) string {
	typeName := getTypeNameWithPackage(t)

	var fields []string
	indent := strings.Repeat("    ", depth)
	nextIndent := strings.Repeat("    ", depth+1)

	for i := 0; i < v.NumField(); i++ {
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

			// Check if the actual value is a pointer to a struct
			if actualType.Kind() == reflect.Pointer && !actualValue.IsNil() {
				fieldLiteral := protoToLiteralRecursive(actualValue, actualType, depth+1)
				fields = append(
					fields,
					fmt.Sprintf("%s%s: %s", nextIndent, field.Name, fieldLiteral),
				)
			} else {
				fieldLiteral := protoToLiteralRecursive(actualValue, actualType, depth+1)
				fields = append(fields, fmt.Sprintf("%s%s: %s", nextIndent, field.Name, fieldLiteral))
			}
		} else {
			fieldLiteral := protoToLiteralRecursive(fieldValue, field.Type, depth+1)
			fields = append(fields, fmt.Sprintf("%s%s: %s", nextIndent, field.Name, fieldLiteral))
		}
	}

	if len(fields) == 0 {
		return fmt.Sprintf("%s{}", typeName)
	}

	return fmt.Sprintf("%s{\n%s,\n%s}", typeName, strings.Join(fields, ",\n"), indent)
}

func getTypeNameWithPackage(t reflect.Type) string {
	// Get the base type name
	typeName := t.Name()
	if typeName == "" {
		// For types like slices, maps, pointers, use the full string representation
		fullName := t.String()
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

	// Get package path
	pkgPath := t.PkgPath()
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

func isPrimitiveType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	}
	return false
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
	}
	return false
}

func isInternalProtoField(name string) bool {
	return strings.HasPrefix(name, "XXX_") ||
		strings.HasPrefix(name, "state") ||
		strings.HasPrefix(name, "sizeCache") ||
		strings.HasPrefix(name, "unknownFields") ||
		name == "UnknownFields"
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0.0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Slice, reflect.Array:
		return v.Len() == 0
	case reflect.Map:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}
