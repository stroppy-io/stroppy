package tpch

import (
	"embed"
	"io/fs"

	"github.com/stroppy-io/stroppy/v6/internal/author"
)

//go:embed *.go LICENSE *.sql README.md published *.json dbgen tpchgen
var published embed.FS

func publication(descriptor string) fs.FS { return author.Builtin(published, "tpch", descriptor) }
