package author

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
)

// relocateSourcePackage keeps repeat ejection aligned with the restored package.
//
//nolint:gocognit,cyclop // only publication metadata in typed Test literals is relocated.
func relocateSourcePackage(data []byte, origin, destination string) ([]byte, error) {
	if origin == "" || origin == destination {
		return data, nil
	}

	set := token.NewFileSet()

	file, err := parser.ParseFile(set, "source.go", data, 0)
	if err != nil {
		return nil, err
	}

	aliases := map[string]bool{}

	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}

		if importPath == SDK+"/pkg/bench" {
			alias := "bench"
			if spec.Name != nil {
				alias = spec.Name.Name
			}

			aliases[alias] = true
		}
	}

	literals := []*ast.BasicLit{}

	ast.Inspect(file, func(node ast.Node) bool {
		value, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}

		typ, ok := value.Type.(*ast.SelectorExpr)
		if !ok || typ.Sel.Name != "Test" {
			return true
		}

		qualifier, ok := typ.X.(*ast.Ident)
		if !ok || !aliases[qualifier.Name] {
			return true
		}

		for _, element := range value.Elts {
			pair, pairOK := element.(*ast.KeyValueExpr)
			if !pairOK {
				continue
			}

			key, keyOK := pair.Key.(*ast.Ident)
			if !keyOK || key.Name != "SourcePackage" {
				continue
			}

			literal, literalOK := pair.Value.(*ast.BasicLit)
			if !literalOK || literal.Kind != token.STRING {
				continue
			}

			text, err := strconv.Unquote(literal.Value)
			if err == nil && (text == origin || strings.HasPrefix(text, origin+"/")) {
				literals = append(literals, literal)
			}
		}

		return true
	})
	slices.SortFunc(literals, func(a, b *ast.BasicLit) int { return int(b.Pos() - a.Pos()) })

	for _, literal := range literals {
		start, end := set.Position(literal.Pos()).Offset, set.Position(literal.End()).Offset
		out := append([]byte(nil), data[:start]...)
		text, _ := strconv.Unquote(literal.Value)
		out = append(out, strconv.Quote(destination+strings.TrimPrefix(text, origin))...)
		data = append(out, data[end:]...)
	}

	return data, nil
}
