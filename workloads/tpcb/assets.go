package tpcb

import (
	"embed"

	"github.com/stroppy-io/stroppy/v6/workloads"
)

//go:embed *.sql README.md
var files embed.FS

func init() {
	workloads.Register(workloads.PresetTPCB, files)
}
