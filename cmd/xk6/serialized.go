package xk6

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func protoNew[T proto.Message]() (model T) { //nolint: ireturn,nonamedreturns // allow
	return model.ProtoReflect().Type().New().Interface().(T) //nolint: errcheck,forcetypeassert // allow
}

type Serialized[T proto.Message] string

func (s Serialized[T]) Unmarshal() (T, error) { //nolint:ireturn
	instance := protoNew[T]()
	err := protojson.Unmarshal([]byte(s), instance)

	return instance, err
}

func MarshalSerialized[T proto.Message](s T) (string, error) {
	b, err := protojson.Marshal(s)

	return string(b), err
}
