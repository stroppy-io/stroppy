package runner

import (
	"fmt"
	"sync/atomic"
)

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("k6 exited with code %d", e.Code) }

var (
	captureK6Exit atomic.Bool
	exitCode      atomic.Int32
)

func init() {
	exitCode.Store(-1)
}

func BeginK6ExitCapture() func() {
	exitCode.Store(-1)
	captureK6Exit.Store(true)

	return func() {
		captureK6Exit.Store(false)
	}
}

func K6ExitCaptureEnabled() bool {
	return captureK6Exit.Load()
}

func exitCodeToError() error {
	switch code := int(exitCode.Load()); code {
	case -1:
		panic("unreachable; k6 must to set an exit code")
	case 0: // do nothing, all correct
		return nil
	default:
		return &ExitError{Code: code}
	}
}

func OSExit(i int) {
	exitCode.Store(int32(i))
}
