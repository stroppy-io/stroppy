package kv

import (
	"testing"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

func TestEvalKv(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		kvMap    *panel.KV_Map
		expected string
	}{
		{
			name:   "single replacement",
			target: "Hello ${NAME}!",
			kvMap: &panel.KV_Map{
				Kvs: []*panel.KV{
					{Key: "NAME", Value: "World"},
				},
			},
			expected: "Hello World!",
		},
		{
			name:   "multiple replacements",
			target: "Hello ${NAME}, welcome to ${PLACE}!",
			kvMap: &panel.KV_Map{
				Kvs: []*panel.KV{
					{Key: "NAME", Value: "John"},
					{Key: "PLACE", Value: "Moscow"},
				},
			},
			expected: "Hello John, welcome to Moscow!",
		},
		{
			name:   "same key multiple times",
			target: "${USER} and ${USER} are friends",
			kvMap: &panel.KV_Map{
				Kvs: []*panel.KV{
					{Key: "USER", Value: "Alice"},
				},
			},
			expected: "Alice and Alice are friends",
		},
		{
			name:   "no matches",
			target: "Hello World!",
			kvMap: &panel.KV_Map{
				Kvs: []*panel.KV{
					{Key: "NAME", Value: "John"},
				},
			},
			expected: "Hello World!",
		},
		{
			name:   "empty kvMap",
			target: "Hello ${NAME}!",
			kvMap: &panel.KV_Map{
				Kvs: []*panel.KV{},
			},
			expected: "Hello ${NAME}!",
		},
		{
			name:     "empty target",
			target:   "",
			kvMap:    &panel.KV_Map{Kvs: []*panel.KV{{Key: "NAME", Value: "John"}}},
			expected: "",
		},
		{
			name:   "STROPPY_ prefix keys",
			target: "Config: ${STROPPY_HOST}:${STROPPY_PORT}",
			kvMap: &panel.KV_Map{
				Kvs: []*panel.KV{
					{Key: "STROPPY_HOST", Value: "localhost"},
					{Key: "STROPPY_PORT", Value: "8080"},
				},
			},
			expected: "Config: localhost:8080",
		},
		{
			name:   "partial match not replaced",
			target: "Value: ${PARTIAL",
			kvMap: &panel.KV_Map{
				Kvs: []*panel.KV{
					{Key: "PARTIAL", Value: "replaced"},
				},
			},
			expected: "Value: ${PARTIAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with string
			result, err := EvalKv(tt.target, tt.kvMap)
			if err != nil {
				t.Errorf("EvalKv() error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("EvalKv() = %v, want %v", result, tt.expected)
			}

			// Test with []byte
			resultBytes, err := EvalKv([]byte(tt.target), tt.kvMap)
			if err != nil {
				t.Errorf("EvalKv([]byte) error = %v", err)
				return
			}
			if string(resultBytes) != tt.expected {
				t.Errorf("EvalKv([]byte) = %v, want %v", string(resultBytes), tt.expected)
			}
		})
	}
}

func TestExtractKvValues(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		expected []string
	}{
		{
			name:     "single STROPPY_ key",
			target:   "Hello ${STROPPY_NAME}!",
			expected: []string{"STROPPY_NAME"},
		},
		{
			name:     "multiple STROPPY_ keys",
			target:   "Config: ${STROPPY_HOST}:${STROPPY_PORT}",
			expected: []string{"STROPPY_HOST", "STROPPY_PORT"},
		},
		{
			name:     "duplicate keys",
			target:   "${STROPPY_USER} and ${STROPPY_USER}",
			expected: []string{"STROPPY_USER", "STROPPY_USER"},
		},
		{
			name:     "mixed STROPPY_ and non-STROPPY_ keys",
			target:   "${STROPPY_VAR1} and ${OTHER_VAR} and ${STROPPY_VAR2}",
			expected: []string{"STROPPY_VAR1", "STROPPY_VAR2"},
		},
		{
			name:     "no STROPPY_ keys",
			target:   "${NAME} and ${PLACE}",
			expected: []string{},
		},
		{
			name:     "empty string",
			target:   "",
			expected: []string{},
		},
		{
			name:     "no placeholders",
			target:   "Just plain text",
			expected: []string{},
		},
		{
			name:     "STROPPY_ with underscores",
			target:   "${STROPPY_MY_LONG_VAR_NAME}",
			expected: []string{"STROPPY_MY_LONG_VAR_NAME"},
		},
		{
			name:     "STROPPY_ with numbers",
			target:   "${STROPPY_VAR123}",
			expected: []string{"STROPPY_VAR123"},
		},
		{
			name:     "incomplete placeholder",
			target:   "${STROPPY_INCOMPLETE",
			expected: []string{},
		},
		{
			name:     "nested braces",
			target:   "${STROPPY_{VAR}}",
			expected: []string{"STROPPY_{VAR"},
		},
		{
			name:     "multiple on same line",
			target:   "a=${STROPPY_A}, b=${STROPPY_B}, c=${STROPPY_C}",
			expected: []string{"STROPPY_A", "STROPPY_B", "STROPPY_C"},
		},
		{
			name:     "multiline text",
			target:   "line1: ${STROPPY_VAR1}\nline2: ${STROPPY_VAR2}\nline3: no var",
			expected: []string{"STROPPY_VAR1", "STROPPY_VAR2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with string
			result := ExtractKvValues(tt.target)
			if !slicesEqual(result.Keys, tt.expected) {
				t.Errorf("ExtractKvValues() = %v, want %v", result.Keys, tt.expected)
			}

			// Test with []byte
			resultBytes := ExtractKvValues([]byte(tt.target))
			if !slicesEqual(resultBytes.Keys, tt.expected) {
				t.Errorf("ExtractKvValues([]byte) = %v, want %v", resultBytes.Keys, tt.expected)
			}
		})
	}
}

func TestExtractKvValues_EmptyResult(t *testing.T) {
	result := ExtractKvValues("no variables here")
	if result == nil {
		t.Error("ExtractKvValues() should not return nil")
	}
	if len(result.Keys) != 0 {
		t.Errorf("ExtractKvValues().Keys should be empty, got %v", result.Keys)
	}
}

func TestEvalKv_WithExtractedKeys(t *testing.T) {
	// Integration test: extract keys and then evaluate
	template := "Server: ${STROPPY_HOST}:${STROPPY_PORT}, User: ${STROPPY_USER}"

	// Extract keys
	keys := ExtractKvValues(template)
	expectedKeys := []string{"STROPPY_HOST", "STROPPY_PORT", "STROPPY_USER"}
	if !slicesEqual(keys.Keys, expectedKeys) {
		t.Errorf("ExtractKvValues() = %v, want %v", keys.Keys, expectedKeys)
	}

	// Evaluate with values
	kvMap := &panel.KV_Map{
		Kvs: []*panel.KV{
			{Key: "STROPPY_HOST", Value: "localhost"},
			{Key: "STROPPY_PORT", Value: "8080"},
			{Key: "STROPPY_USER", Value: "admin"},
		},
	}

	result, err := EvalKv(template, kvMap)
	if err != nil {
		t.Errorf("EvalKv() error = %v", err)
		return
	}

	expected := "Server: localhost:8080, User: admin"
	if result != expected {
		t.Errorf("EvalKv() = %v, want %v", result, expected)
	}
}

// Helper function to compare slices
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
