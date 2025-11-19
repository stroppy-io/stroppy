package protohelp

import "google.golang.org/protobuf/proto"

func ProtoNew[T proto.Message]() (model T) {
	return model.ProtoReflect().Type().New().Interface().(T)
}
