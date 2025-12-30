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
	PackageJSONFileName     FileName = "package.json"
	ProtoTSFileName         FileName = "stroppy.pb.ts"
	ProtoJSFileName         FileName = "stroppy.pb.js"
	K6PluginFileName        FileName = "stroppy-k6"
	ApplyGeneratorsFileName FileName = "apply_generators.ts"
	HelpersFileName         FileName = "helpers.ts"
	ParseSQLFileName        FileName = "parse_sql.ts"
	ParseSQL2TSFileName     FileName = "parse_sql_2.ts"
	ParseSQL2JSFileName     FileName = "parse_sql_2.js"
	AnalyzeDDLTSFileName    FileName = "analyze_ddl.ts"
	AnalyzeDDLJSFileName    FileName = "analyze_ddl.js"
)

// StaticFiles are copied to temp dir for k6 execution.
var StaticFiles = []FileName{ //nolint: gochecknoglobals
	ProtoJSFileName,
	K6PluginFileName,
	HelpersFileName,
	ParseSQLFileName,
	ParseSQL2JSFileName,
	AnalyzeDDLJSFileName,
	ApplyGeneratorsFileName,
}

// DevStaticFiles are additional files for development environment.
var DevStaticFiles = []FileName{ //nolint: gochecknoglobals
	PackageJSONFileName,
	AnalyzeDDLTSFileName,
	ParseSQL2TSFileName,
	ProtoTSFileName,
}

var Binaries = []FileName{ //nolint: gochecknoglobals
	K6PluginFileName,
}

//go:embed package.json
//go:embed stroppy-k6
//go:embed *.js
//go:embed *.ts
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

func CopyAllStaticFilesToPath(targetPath string, perm os.FileMode) error {
	return CopyStaticFilesToPath(targetPath, perm, StaticFiles...)
}
