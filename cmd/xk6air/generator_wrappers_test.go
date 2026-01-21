package xk6air

import (
	"reflect"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func TestGenerationRuleWrapperGenable_GetGenerationRule(t *testing.T) {
	genRuleStruct := stroppy.Generation_Rule{
		Kind:           &stroppy.Generation_Rule_BoolConst{BoolConst: false},
		Distribution:   &stroppy.Generation_Distribution{},
		NullPercentage: new(uint32),
		Unique:         new(bool),
	}
	wrapped := (*GenerationRuleWrapperGenable)(&genRuleStruct)

	tests := []struct {
		name string
		w    *GenerationRuleWrapperGenable
		want *stroppy.Generation_Rule
	}{
		{
			name: "simple",
			w:    wrapped,
			want: &genRuleStruct,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.w.GetGenerationRule(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf(
					"GenerationRuleWrapperGenable.GetGenerationRule() = %v, want %v",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestGenerationRuleWrapperGenable_GetName(t *testing.T) {
	tests := []struct {
		name string
		w    *GenerationRuleWrapperGenable
		want string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.w.GetName(); got != tt.want {
				t.Errorf("GenerationRuleWrapperGenable.GetName() = %v, want %v", got, tt.want)
			}
		})
	}
}
