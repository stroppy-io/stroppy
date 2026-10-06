package author

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
)

const localImports = "stroppy.local/workload"

// Builtin publishes owned source with imports relative to the restored project.
func Builtin(source fs.FS, origin, descriptor string) fs.FS {
	return &builtinSource{FS: source, origin: origin, descriptor: descriptor}
}

type builtinSource struct {
	fs.FS
	origin, descriptor string
}

func (s *builtinSource) Open(name string) (fs.File, error) {
	file, err := s.FS.Open(name)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()

		return nil, err
	}

	if info.IsDir() {
		return file, nil
	}

	_ = file.Close()

	data, err := s.ReadFile(name)
	if err != nil {
		return nil, err
	}

	return &publishedFile{Reader: bytes.NewReader(data), info: info}, nil
}

type publishedFile struct {
	*bytes.Reader
	info fs.FileInfo
}

func (f *publishedFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (*publishedFile) Close() error                 { return nil }

//nolint:gocognit,cyclop,funlen // published builtins retain local imports, assets and selected registration.
func (s *builtinSource) ReadFile(name string) ([]byte, error) {
	data, err := fs.ReadFile(s.FS, name)
	if err != nil {
		return nil, err
	}

	if !strings.HasSuffix(name, ".go") {
		return data, nil
	}

	data = bytes.ReplaceAll(data, []byte(SDK+"/workloads/"+s.origin), []byte(localImports))
	data = bytes.ReplaceAll(data,
		[]byte(SDK+"/workloads/internal/workloadtest"), []byte(localImports+"/published/workloadtest"))

	data = bytes.ReplaceAll(data, []byte(SDK+"/pkg/datagen/source"), []byte(localImports+"/published/source"))
	if strings.Contains(name, "/") {
		return data, nil
	}

	if strings.HasSuffix(name, "_test.go") {
		if s.origin == "tpcb" && name == "procs_test.go" {
			data = append(data, []byte("\nfunc init() { bench.Register("+siblingDescriptor(s.descriptor)+") }\n")...)
		}

		return data, nil
	}

	set := token.NewFileSet()

	file, err := parser.ParseFile(set, name, data, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	if name == "publication.go" {
		patterns := ""

		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "//go:embed ") {
				patterns = strings.TrimPrefix(line, "//go:embed ")
			}
		}

		return []byte("package " + file.Name.Name + "\n\nimport (\"embed\"; \"io/fs\")\n\n//go:embed " + patterns +
			"\nvar published embed.FS\n\nfunc publication(string) fs.FS { return published }\n"), nil
	}

	if name == "assets.go" {
		return []byte("package " + file.Name.Name + "\n\nimport \"embed\"\n\n//go:embed " +
			assetPatterns(s.origin) + "\nvar files embed.FS\n"), nil
	}
	// An ejected variant registers its selected descriptor, not its siblings.
	for _, declaration := range file.Decls {
		function, functionOK := declaration.(*ast.FuncDecl)
		if !functionOK || function.Name.Name != "init" {
			continue
		}

		kept := []ast.Stmt{}

		for _, statement := range function.Body.List {
			expression, ok := statement.(*ast.ExprStmt)
			if !ok {
				kept = append(kept, statement)

				continue
			}

			call, ok := expression.X.(*ast.CallExpr)
			if !ok {
				kept = append(kept, statement)

				continue
			}

			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Register" {
				kept = append(kept, statement)

				continue
			}

			receiver, ok := selector.X.(*ast.Ident)
			if !ok || receiver.Name != "bench" {
				kept = append(kept, statement)

				continue
			}

			if len(call.Args) == 1 {
				value, valueOK := call.Args[0].(*ast.Ident)
				if valueOK && value.Name == s.descriptor {
					kept = append(kept, statement)
				}
			}
		}

		function.Body.List = kept
	}

	var output bytes.Buffer
	if err := format.Node(&output, set, file); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}

func siblingDescriptor(selected string) string {
	if selected == "Tx" {
		return "Procs"
	}

	return "Tx"
}

func assetPatterns(origin string) string {
	if origin == "tpch" || origin == "tpcds" {
		return "*.sql *.json README.md"
	}

	return "*.sql README.md"
}
