// Package author scaffolds standalone projects from explicitly published workload files.
package author

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

const (
	SDK      = "github.com/stroppy-io/stroppy/v6"
	maxFiles = 4096
	maxBytes = 128 << 20
)

var (
	ErrSourceUnavailable = errors.New("author did not publish source")
	ErrInvalidSource     = errors.New("invalid published source")
)

// Read validates and copies only explicitly published regular files.
//
//nolint:gocognit // validate every published entry before copying its bytes.
func Read(source fs.FS) (map[string][]byte, error) {
	if source == nil {
		return nil, ErrSourceUnavailable
	}

	files := map[string][]byte{}
	total := 0

	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if name == "." {
			return nil
		}

		if !fs.ValidPath(name) || strings.Contains(name, "\\") || strings.Contains(name, ":") {
			return fmt.Errorf("%w: path %q", ErrInvalidSource, name)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: nonregular %q", ErrInvalidSource, name)
		}

		if len(files) >= maxFiles || info.Size() > int64(maxBytes-total) {
			return fmt.Errorf("%w: publication too large", ErrInvalidSource)
		}

		file, err := source.Open(name)
		if err != nil {
			return err
		}

		data, readErr := io.ReadAll(io.LimitReader(file, int64(maxBytes-total)+1))

		closeErr := file.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}

		total += len(data)
		if total > maxBytes {
			return fmt.Errorf("%w: publication too large", ErrInvalidSource)
		}

		files[name] = data

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, ErrSourceUnavailable
	}

	return files, nil
}

// Project creates root main/module files and restores package-relative publication.
//
//nolint:gocognit // project scaffold and authored package remain one result.
func Project(name, modulePath, version string, published map[string][]byte) (map[string][]byte, error) {
	if err := module.CheckPath(modulePath); err != nil {
		return nil, err
	}

	files := map[string][]byte{}

	if _, project := published["go.mod"]; project {
		return publishedProject(modulePath, version, published)
	}

	for name, data := range published {
		if strings.HasSuffix(name, ".go") {
			data = bytes.ReplaceAll(data, []byte(localImports), []byte(modulePath+"/workload"))
		}

		destination := path.Join("workload", name)
		files[destination] = data
	}

	packageName := ""

	for name, data := range published {
		if path.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), name, data, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}

		if path.Dir(name) == "." {
			packageName = parsed.Name.Name
		}
	}

	if packageName == "" || packageName == "main" {
		return nil, fmt.Errorf("%w: importable Go package missing", ErrInvalidSource)
	}

	descriptor, err := Descriptor(published, name)
	if err != nil {
		return nil, err
	}

	main := fmt.Sprintf("package main\n\nimport (\n stroppy %q\n workload %q\n)\n\n"+
		"func main() { stroppy.Main(workload.%s) }\n", SDK, modulePath+"/workload", descriptor)

	formatted, err := format.Source([]byte(main))
	if err != nil {
		return nil, err
	}

	files["main.go"] = formatted

	moduleFile := new(modfile.File)
	if err := moduleFile.AddModuleStmt(modulePath); err != nil {
		return nil, err
	}

	if err := moduleFile.AddGoStmt("1.27"); err != nil {
		return nil, err
	}

	if err := moduleFile.AddRequire(SDK, version); err != nil {
		return nil, err
	}

	moduleData, err := moduleFile.Format()
	if err != nil {
		return nil, err
	}

	files["go.mod"] = moduleData

	files["README.md"] = []byte(fmt.Sprintf("# %s\n\nRequires Go 1.27 or newer.\n\n"+
		"```sh\ngo run .\ngo test ./...\nstroppy build .\n```\n\n"+
		"Published files are restored under workload/. Dependencies declared by those files\n"+
		"are resolved through Go modules. Custom workloads are trusted native code, not sandboxed.\n", name))
	if license, ok := published["LICENSE"]; ok {
		files["LICENSE"] = license
	}

	return files, nil
}

// Descriptor finds the exported Test descriptor matching a workload's registered name.
//
//nolint:gocognit,cyclop // descriptor literals are inspected without executing author code.
func Descriptor(files map[string][]byte, name string) (string, error) {
	candidates := []string{}
	conventional := false

	for filename, data := range files {
		if path.Dir(filename) != "." || path.Ext(filename) != ".go" || strings.HasSuffix(filename, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(token.NewFileSet(), filename, data, 0)
		if err != nil {
			return "", err
		}

		for _, top := range file.Decls {
			declaration, ok := top.(*ast.GenDecl)
			if !ok || declaration.Tok != token.VAR {
				continue
			}

			for _, spec := range declaration.Specs {
				value, valueOK := spec.(*ast.ValueSpec)
				if !valueOK {
					continue
				}

				for _, identifier := range value.Names {
					if identifier.Name == "Test" {
						conventional = true
					}
				}
			}
		}

		ast.Inspect(file, func(node ast.Node) bool {
			declaration, declarationOK := node.(*ast.ValueSpec)
			if !declarationOK {
				return true
			}

			for i, value := range declaration.Values {
				literal, literalOK := value.(*ast.CompositeLit)
				if !literalOK || i >= len(declaration.Names) {
					continue
				}

				selector, ok := literal.Type.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Test" {
					continue
				}

				for _, element := range literal.Elts {
					pair, pairOK := element.(*ast.KeyValueExpr)
					if !pairOK {
						continue
					}

					key, keyOK := pair.Key.(*ast.Ident)
					if !keyOK || key.Name != "Name" {
						continue
					}

					text, textOK := pair.Value.(*ast.BasicLit)
					if !textOK {
						continue
					}

					registered, _ := strconv.Unquote(text.Value)
					if registered == name && ast.IsExported(declaration.Names[i].Name) {
						candidates = append(candidates, declaration.Names[i].Name)
					}
				}
			}

			return true
		})
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}
	// The conventional Test value may compute its identity in ordinary Go.
	if conventional {
		return "Test", nil
	}

	return "", fmt.Errorf("%w: exported Test descriptor for %q not found", ErrInvalidSource, name)
}
