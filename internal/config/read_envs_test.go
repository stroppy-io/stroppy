package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestMutateConfigByEnvs(t *testing.T) {
	testVersion := "123.0.0"

	t.Setenv("CONFIG__GLOBAL__VERSION", testVersion)

	cfg := NewExampleConfig()

	err := updateConfigWithDirectEnvs(cfg)
	if err != nil {
		t.Error(err)

		return
	}

	require.Equal(t, cfg.GetGlobal().GetVersion(), testVersion)
}

func Example_updateConfigWithDirectEnvs() { //nolint: testableexamples // not reproduceble
	cfg := NewExampleConfig()

	var names []string

	traverseMessage(cfg.ProtoReflect(),
		func(msg protoreflect.Message, field protoreflect.FieldDescriptor, value protoreflect.Value) {
			names = append(names, field.JSONName())

			if field.Kind() != protoreflect.MessageKind {
				var isListStr string
				if field.IsList() {
					isListStr = "[]"
				}

				fmt.Printf("%v :: %s%s :: %s \n", names, isListStr, field.Kind().String(), value.String())

				if field.Kind() == protoreflect.StringKind && !field.IsList() {
					msg.Set(field, protoreflect.ValueOfString("Mutated value"))
				}
			}
		},
		func(_ protoreflect.Message, _ protoreflect.FieldDescriptor, _ protoreflect.Value) {
			names = names[:len(names)-1]
		})

	traverseMessage(cfg.ProtoReflect(),
		func(_ protoreflect.Message, field protoreflect.FieldDescriptor, value protoreflect.Value) {
			names = append(names, field.JSONName())

			if field.Kind() != protoreflect.MessageKind {
				var isListStr string
				if field.IsList() {
					isListStr = "[]"
				}

				fmt.Printf("%v :: %s%s :: %s \n", names, isListStr, field.Kind().String(), value.String())
			}
		},
		func(_ protoreflect.Message, _ protoreflect.FieldDescriptor, _ protoreflect.Value) {
			names = names[:len(names)-1]
		})
}
