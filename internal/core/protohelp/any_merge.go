package protohelp

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

func CopyCommonFields(src, dst proto.Message) error {
	if src == nil || dst == nil {
		return fmt.Errorf("src and dst must not be nil")
	}

	srcMsg := src.ProtoReflect()
	dstMsg := dst.ProtoReflect()

	srcMsg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		dstField := dstMsg.Descriptor().Fields().ByName(fd.Name())
		if dstField == nil {
			// dst doesn't have this field, skip
			return true
		}

		// Check basic compatibility
		if fd.Kind() != dstField.Kind() ||
			fd.IsList() != dstField.IsList() ||
			fd.IsMap() != dstField.IsMap() {
			return true
		}

		// For message kinds, ensure they are the same type
		if fd.Kind() == protoreflect.MessageKind {
			if fd.Message().FullName() != dstField.Message().FullName() {
				return true
			}
		}

		// Only copy if src has it set; otherwise you might want to clear on dst.
		if !srcMsg.Has(fd) {
			dstMsg.Clear(dstField)
			return true
		}

		// Shallow copy
		dstMsg.Set(dstField, v)
		return true
	})

	return nil
}

func CopyCommonFieldsFromAnypbMessages(src, dst *anypb.Any) (*anypb.Any, error) {
	srcStruct, err := src.UnmarshalNew()
	if err != nil {
		return nil, err
	}
	dstStruct, err := dst.UnmarshalNew()
	if err != nil {
		return nil, err
	}
	err = CopyCommonFields(srcStruct, dstStruct)
	return anypb.New(dstStruct)
}
