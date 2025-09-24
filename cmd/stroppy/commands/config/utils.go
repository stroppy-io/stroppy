package config

import (
	"fmt"
	"path"
	"slices"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/internal/config"
	"github.com/stroppy-io/stroppy/internal/protoyaml"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

const (
	DefaultConfigName   = "stroppy"
	DefaultConfigFormat = config.FormatJSON
)

const (
	yamlAnnotation = "# yaml-language-server: $schema=https://github.com/stroppy-io/stroppy-proto/releases/download/" +
		stroppy.Version + "/config.schema.yaml\n"
	jsonAnnotation = "\t" + `"$schema": "https://github.com/stroppy-io/stroppy-proto/releases/download/` +
		stroppy.Version + `/config.schema.json",` + "\n"
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

	switch path.Ext(newPath) { // embed annotations into config file
	case ".json":
		configStr = slices.Concat(configStr[:2], []byte(jsonAnnotation), configStr[2:])
	case ".yaml":
		configStr = slices.Concat([]byte(yamlAnnotation), configStr)
	}

	return configStr, nil
}
