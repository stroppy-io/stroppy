package author

import (
	"fmt"
	"strings"

	"golang.org/x/mod/modfile"
)

func publishedProject(modulePath, version string, published map[string][]byte) (map[string][]byte, error) {
	moduleFile, err := modfile.Parse("go.mod", published["go.mod"], nil)
	if err != nil {
		return nil, err
	}

	if moduleFile.Module == nil {
		return nil, fmt.Errorf("%w: published module path missing", ErrInvalidSource)
	}

	if _, exists := published["main.go"]; !exists {
		return nil, fmt.Errorf("%w: complete project publication requires main.go", ErrInvalidSource)
	}

	files := map[string][]byte{}

	for filename, data := range published {
		if strings.HasSuffix(filename, ".go") {
			data, err = rewriteImports(data, moduleFile.Module.Mod.Path, modulePath)
			if err != nil {
				return nil, err
			}

			data, err = relocateSourcePackage(data, moduleFile.Module.Mod.Path, modulePath)
			if err != nil {
				return nil, err
			}
		}

		files[filename] = data
	}

	if err := moduleFile.AddModuleStmt(modulePath); err != nil {
		return nil, err
	}

	if err := moduleFile.AddRequire(SDK, version); err != nil {
		return nil, err
	}

	data, err := moduleFile.Format()
	if err != nil {
		return nil, err
	}

	files["go.mod"] = data

	return files, nil
}
