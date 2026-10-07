package tpcds

import (
	"embed"
	"io/fs"

	"github.com/stroppy-io/stroppy/v6/internal/author"
)

//go:embed *.go LICENSE *.sql README.md published *.json dsdgen dsqgen tpcdsgen
var published embed.FS

func publication(descriptor string) fs.FS { return author.Builtin(published, "tpcds", descriptor) }
