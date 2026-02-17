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
	StroppyDTSFileName FileName = "stroppy.d.ts"

	HelpersFileName FileName = "helpers.ts"

	ParseSQLTSFileName FileName = "parse_sql.ts"
	ParseSQLJSFileName FileName = "parse_sql.js"

	ProtoTSFileName FileName = "stroppy.pb.ts"
	ProtoJSFileName FileName = "stroppy.pb.js"

	PackageJSONFileName FileName = "package.json"
	TSConfigFileName    FileName = "tsconfig.json"
)

// StaticFiles are copied to temp dir for k6 execution.
var StaticFiles = []FileName{
	ProtoJSFileName,
	HelpersFileName,
	ParseSQLJSFileName,
	StroppyDTSFileName,
}

// DevStaticFiles are additional files for development environment.
var DevStaticFiles = []FileName{
	PackageJSONFileName,
	ParseSQLTSFileName,
	ProtoTSFileName,
	TSConfigFileName,
}

var Binaries = []FileName{}

//go:embed package.json
//go:embed *.js
//go:embed *.ts
//go:embed *.d.ts
//go:embed *.pb.*
//go:embed tsconfig.json
var Content embed.FS

func CopyStaticFilesToPath(targetPath string, perm os.FileMode, files ...FileName) error {
	for _, name := range files {
		data, err := Content.ReadFile(string(name))
		if err != nil {
			return fmt.Errorf(
				"failed to open static file name: %s, error: %w",
				name,
				err,
			)
		}

		err = os.WriteFile(path.Join(targetPath, string(name)), data, perm)
		if err != nil {
			return fmt.Errorf("failed to write file: %w", err)
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
