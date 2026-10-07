package bench

import (
	"errors"
	"fmt"
)

// ValidationError identifies invalid author declarations or configuration.
// Convenience APIs panic with this value; application boundaries return it as an error.
type ValidationError struct {
	Operation string
	Cause     error
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %v", e.Operation, e.Cause) }
func (e *ValidationError) Unwrap() error { return e.Cause }

func invalid(operation string, err error) {
	panic(&ValidationError{Operation: operation, Cause: err})
}

//nolint:gocritic // deferred recovery must update the owning error result.
func recoverValidation(err *error) {
	if value := recover(); value != nil {
		if failure, ok := value.(*ValidationError); ok {
			*err = failure

			return
		}

		panic(value)
	}
}

var errInvalidAuthorInput = errors.New("invalid author input")

func inputError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errInvalidAuthorInput, fmt.Sprintf(format, args...))
}
