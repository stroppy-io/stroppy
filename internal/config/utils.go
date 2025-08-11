package config

import (
	"errors"
	"fmt"
	"os"
	"path"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/internal/protoyaml"
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
		return cfg, protojson.Unmarshal(data, cfg)
	case ".yaml", ".yml":
		return cfg, protoyaml.Unmarshal(data, cfg)
	default:
		return cfg, fmt.Errorf("unknown file extension: %s", path.Ext(filePath)) //nolint: err113
	}
}

var (
	ErrRunConfigPathIsEmpty = errors.New("run config path is empty")
	ErrFoundPathIsEmpty     = errors.New("found path is empty")
)

func getRelativePath(runConfigPath, foundPath string) (string, error) {
	if runConfigPath == "" {
		return "", ErrRunConfigPathIsEmpty
	}

	if foundPath == "" {
		return "", ErrFoundPathIsEmpty
	}

	if !path.IsAbs(foundPath) {
		return path.Join(path.Dir(runConfigPath), foundPath), nil
	}

	return foundPath, nil
}

var (
	ErrPathIsEmpty          = errors.New("path is empty")
	ErrPathIsNotRegularFile = errors.New("path is not a regular file")
	ErrPathIsNotExecutable  = errors.New("path is not executable")
)

func validatePath(path string, executable bool) error {
	if path == "" {
		return ErrPathIsEmpty
	}

	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if !stat.Mode().IsRegular() {
		return ErrPathIsNotRegularFile
	}

	if executable {
		if stat.Mode()&0o111 == 0 {
			return ErrPathIsNotExecutable
		}
	}

	return nil
}
