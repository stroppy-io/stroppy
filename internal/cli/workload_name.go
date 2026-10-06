package cli

import (
	"errors"
	"fmt"
)

var errReservedWorkloadName = errors.New("stroppy: reserved workload name")

func ValidateWorkloadName(name string) error {
	switch name {
	case "build", "cache", "export", "help", "init", "eject", "list", "probe", "remove", "run", "version", "completion":
		return fmt.Errorf("%w %q", errReservedWorkloadName, name)
	default:
		return nil
	}
}
