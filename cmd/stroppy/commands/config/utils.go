package config

import (
	"fmt"
	"path"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"

	"github.com/stroppy-io/stroppy/internal/config"
	"github.com/stroppy-io/stroppy/internal/protoyaml"
)

const (
	DefaultConfigName   = "stroppy"
	DefaultConfigFormat = config.FormatJSON
)

func MarshalConfig(cfg *stroppy.Config, newPath string) ([]byte, error) {
	var marshaler func(m proto.Message) ([]byte, error)

	switch path.Ext(newPath) {
	case ".json":
		marshaler = protojson.MarshalOptions{Indent: "\t"}.Marshal
	case ".yaml":
		marshaler = protoyaml.Marshal
	default:
		return nil, fmt.Errorf("unknown file extension: %s", path.Ext(newPath)) //nolint: err113
	}

	configStr, err := marshaler(cfg)
	if err != nil {
		return nil, err
	}

	return configStr, nil
}
