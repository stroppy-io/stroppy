package xk6

import (
	"go.k6.io/k6/js/modules"
)

type RootModule struct{}

func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance { //nolint:ireturn
	return NewXK6Instance(vu, vu.Runtime().NewObject())
}

var _ modules.Module = new(RootModule)

func init() { //nolint:gochecknoinits // allow for xk6
	modules.Register("k6/x/stroppy", new(RootModule))
}
