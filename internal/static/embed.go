package static

import (
	"embed"
	"fmt"
	"os"
	"path"
	"slices"

	"github.com/stroppy-io/stroppy/internal/common"
)

type FileName string

func (f FileName) String() string {
	return string(f)
}

const (
	PackageJSONFileName FileName = "package.json"
	ProtoTSFileName     FileName = "stroppy.pb.ts"
	ProtoJSFileName     FileName = "stroppy.pb.js"
	K6PluginFileName    FileName = "stroppy-xk6"
	K6BenchmarkFileName FileName = "benchmark.ts"
)

var StaticFiles = []FileName{ //nolint: gochecknoglobals
	ProtoJSFileName,
	K6PluginFileName,
	K6BenchmarkFileName,
}

var DevStaticFiles = []FileName{ //nolint: gochecknoglobals
	PackageJSONFileName,
	ProtoTSFileName,
}

var Binaries = []FileName{ //nolint: gochecknoglobals
	K6PluginFileName,
}

//go:embed package.json
//go:embed stroppy-xk6
//go:embed benchmark.ts
//go:embed *.pb.*
var Content embed.FS

func openFile(name FileName) ([]byte, error) {
	file, err := Content.Open(string(name))
	if err != nil {
		return nil, err
	}

	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, stat.Size())

	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

func CopyStaticFilesToPath(targetPath string, perm os.FileMode, files ...FileName) error {
	for _, name := range files {
		data, err := openFile(name)
		if err != nil {
			return fmt.Errorf(
				"failed to open static file name: %s, error: %w",
				name,
				err,
			) //nolint: err113
		}

		err = os.WriteFile(path.Join(targetPath, string(name)), data, perm)
		if err != nil {
			return fmt.Errorf("failed to write file: %w", err) //nolint: err113
		}

		if slices.Contains(Binaries, name) {
			err = os.Chmod(path.Join(targetPath, string(name)), common.FolderMode)
			if err != nil {
				return fmt.Errorf("failed to chmod file: %w", err) //nolint: err113
			}
		}
	}

	return nil
}
