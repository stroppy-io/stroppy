package tpcc

import (
	"embed"

	"github.com/stroppy-io/stroppy/workloads"
)

//go:embed *.sql README.md
var files embed.FS

func init() {
	workloads.Register(workloads.PresetTPCC, files)
}
