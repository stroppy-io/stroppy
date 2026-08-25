package tpcds

import (
	"embed"

	"github.com/stroppy-io/stroppy/workloads"
)

//go:embed *.sql *.json README.md
var files embed.FS

func init() {
	workloads.Register(workloads.PresetTPCDS, files)
}
