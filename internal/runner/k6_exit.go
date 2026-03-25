package runner

import "fmt"

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("k6 exited with code %d", e.Code) }

var exitCode int = -1

func exitCodeToError() error {
	switch exitCode {
	case -1:
		panic("unreachable; k6 must to set an exit code")
	case 0: // do nothing, all correct
		return nil
	default:
		return &ExitError{Code: exitCode}
	}
}

func OSExit(i int) {
	exitCode = i
}
