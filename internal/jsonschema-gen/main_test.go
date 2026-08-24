package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSchemaDescribesFileEnvelope(t *testing.T) {
	schema := buildSchema()
	defs := object(t, schema["$defs"])
	runConfig := object(t, defs["RunConfig"])
	require.Equal(t, false, runConfig["additionalProperties"])

	properties := object(t, runConfig["properties"])
	run := object(t, properties["run"])
	params := object(t, properties["params"])
	require.Equal(t, false, run["additionalProperties"])
	require.NotNil(t, params["additionalProperties"])
	require.NotContains(t, anyOf(t, params["additionalProperties"]), map[string]any{"type": "null"})

	runProperties := object(t, run["properties"])
	require.Contains(t, runProperties, "queryTimeout")
	require.Contains(t, runProperties, "query_timeout")
	require.Equal(t, runProperties["queryTimeout"], runProperties["query_timeout"])

	global := object(t, defs["GlobalConfig"])
	require.NotContains(t, object(t, global["properties"]), "queryTimeout")

	require.Contains(t, properties, "noSteps")
	require.Contains(t, properties, "no_steps")
	require.Equal(t, properties["noSteps"], properties["no_steps"])

	drivers := firstAnyOf(t, properties["drivers"])
	driverValues := object(t, drivers["additionalProperties"])
	require.Equal(t, "#/$defs/DriverRunConfig", driverValues["$ref"])
	require.NotContains(t, driverValues, "anyOf")
}

func TestBuildSchemaDescribesAcceptedScalarForms(t *testing.T) {
	defs := object(t, buildSchema()["$defs"])
	driverProperties := object(t, object(t, defs["DriverRunConfig"])["properties"])
	bulkSize := firstAnyOf(t, driverProperties["bulkSize"])
	bulkForms := anyOf(t, bulkSize)
	require.Len(t, bulkForms, 2)
	require.Equal(t, "number", object(t, bulkForms[0])["type"])
	require.Equal(t, "string", object(t, bulkForms[1])["type"])
	require.Equal(t, true, object(t, bulkForms[1])["x-stroppy-exactInt32"])

	globalProperties := object(t, object(t, defs["GlobalConfig"])["properties"])
	seed := firstAnyOf(t, globalProperties["seed"])
	require.Equal(t, "integer", seed["type"])
	require.Equal(t, true, seed["x-stroppy-bareInteger"])
	require.Equal(t, true, seed["x-stroppy-rejectExponent"])

	loggerProperties := object(t, object(t, defs["LoggerConfig"])["properties"])
	logLevel := firstAnyOf(t, loggerProperties["logLevel"])
	logForms := anyOf(t, logLevel)
	require.Equal(t, "string", object(t, logForms[0])["type"])
	require.Equal(t, []string{
		"LOG_LEVEL_DEBUG",
		"LOG_LEVEL_INFO",
		"LOG_LEVEL_WARN",
		"LOG_LEVEL_ERROR",
		"LOG_LEVEL_FATAL",
	}, stringSlice(t, object(t, logForms[0])["enum"]))
	require.Equal(t, "integer", object(t, logForms[1])["type"])
	require.Equal(t, []int{0, 1, 2, 3, 4}, intSlice(t, object(t, logForms[1])["enum"]))
}

func TestGeneratedSchemaIsCurrentAndDeterministic(t *testing.T) {
	first, err := json.MarshalIndent(buildSchema(), "", "  ")
	require.NoError(t, err)
	second, err := json.MarshalIndent(buildSchema(), "", "  ")
	require.NoError(t, err)
	require.Equal(t, first, second)

	first = append(first, '\n')
	generated, err := os.ReadFile(filepath.Join("..", "..", "docs", "jsonschema", "run.schema.json"))
	require.NoError(t, err)
	require.Equal(t, first, generated, "run go generate ./pkg/config")
}

func object(t *testing.T, value any) map[string]any {
	t.Helper()

	result, ok := value.(map[string]any)
	require.True(t, ok, "got %T", value)

	return result
}

func anyOf(t *testing.T, value any) []any {
	t.Helper()
	result, ok := object(t, value)["anyOf"].([]any)
	require.True(t, ok, "got %T", object(t, value)["anyOf"])

	return result
}

func firstAnyOf(t *testing.T, value any) map[string]any {
	t.Helper()
	forms := anyOf(t, value)
	require.NotEmpty(t, forms)

	return object(t, forms[0])
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()

	values := reflect.ValueOf(value)
	require.Equal(t, reflect.Slice, values.Kind())

	result := make([]string, values.Len())
	for index := range values.Len() {
		result[index] = values.Index(index).Interface().(string)
	}

	return result
}

func intSlice(t *testing.T, value any) []int {
	t.Helper()

	values := reflect.ValueOf(value)
	require.Equal(t, reflect.Slice, values.Kind())

	result := make([]int, values.Len())
	for index := range values.Len() {
		result[index] = values.Index(index).Interface().(int)
	}

	return result
}
