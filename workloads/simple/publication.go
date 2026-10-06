package simple

import (
	"embed"
	"io/fs"

	"github.com/stroppy-io/stroppy/v6/internal/author"
)

//go:embed *.go LICENSE
var published embed.FS

func publication(descriptor string) fs.FS { return author.Builtin(published, "simple", descriptor) }
