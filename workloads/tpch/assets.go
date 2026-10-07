package tpch

import (
	"embed"

	"github.com/stroppy-io/stroppy/v6/workloads"
)

//go:embed *.sql *.json README.md
var files embed.FS

func init() {
	workloads.Register(workloads.PresetTPCH, files)
}
