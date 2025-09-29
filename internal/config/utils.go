package config

import (
	"fmt"
	"os"
	"path"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/pkg/utils/protoyaml"
)

func toPtr[T any](value T) *T {
	return &value
}

func protoNew[T proto.Message]() (model T) { //nolint: ireturn,nonamedreturns // allow
	return model.ProtoReflect().Type().New().Interface().(T) //nolint: errcheck,forcetypeassert // allow
}

func loadProtoConfig[T proto.Message](filePath string) (T, error) { //nolint: ireturn // generic
	cfg := protoNew[T]()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return cfg, err
	}

	switch path.Ext(filePath) {
	case ".json":
		return cfg, protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(data, cfg)
	case ".yaml", ".yml":
		return cfg, protoyaml.Unmarshal(data, cfg)
	default:
		return cfg, fmt.Errorf("unknown file extension: %s", path.Ext(filePath)) //nolint: err113
	}
}
