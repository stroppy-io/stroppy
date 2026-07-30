package engine

import (
	"fmt"

	js "github.com/grafana/sobek"
)

// Mocks is a tiny helper to bulk-set sobek globals.
type Mocks []struct {
	name  string
	value any
}

func (m Mocks) Set(vm *js.Runtime) error {
	for _, kv := range m {
		if err := vm.Set(kv.name, kv.value); err != nil {
			return fmt.Errorf("set %q: %w", kv.name, err)
		}
	}
	return nil
}
