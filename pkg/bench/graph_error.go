package bench

import (
	"context"
	"errors"
)

func graphRunFailure(parent context.Context, cause, err error) error {
	if parent.Err() == nil && cause != nil {
		return withoutGraphCancellation(err)
	}

	return err
}

func withoutGraphCancellation(err error) error {
	if err == nil {
		return nil
	}

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var remaining error
		for _, member := range joined.Unwrap() {
			remaining = errors.Join(remaining, withoutGraphCancellation(member))
		}

		return remaining
	}

	if wrapped, ok := err.(interface{ Unwrap() error }); ok && errors.Is(err, context.Canceled) {
		if remaining := withoutGraphCancellation(wrapped.Unwrap()); remaining != nil {
			if IsFatalError(err) {
				return Fatal(remaining)
			}

			return remaining
		}

		return nil
	}

	if errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}
