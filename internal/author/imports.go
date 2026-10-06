package author

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// rewriteImports relocates a package tree without changing authored code or comments.
func rewriteImports(data []byte, origin, destination string) ([]byte, error) {
	if origin == "" || origin == destination {
		return data, nil
	}

	set := token.NewFileSet()

	file, err := parser.ParseFile(set, "source.go", data, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	for index := len(file.Imports) - 1; index >= 0; index-- {
		literal := file.Imports[index].Path

		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return nil, err
		}

		if value != origin && !strings.HasPrefix(value, origin+"/") {
			continue
		}

		start := set.Position(literal.Pos()).Offset
		end := set.Position(literal.End()).Offset
		replacement := strconv.Quote(destination + strings.TrimPrefix(value, origin))
		out := make([]byte, 0, len(data)+len(replacement)-(end-start))
		out = append(out, data[:start]...)
		out = append(out, replacement...)
		data = append(out, data[end:]...)
	}

	return data, nil
}
