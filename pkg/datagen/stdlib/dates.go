package stdlib

import (
	"fmt"
	"time"
)

// secondsPerDay is the invariant for UTC (no leap seconds in wall-clock
// day arithmetic). Epoch-day semantics treat the calendar as a uniform
// 86400-second grid, which is the TPC spec convention.
const secondsPerDay int64 = 86_400

func init() {
	registry["std.daysToDate"] = daysToDate
	registry["std.dateToDays"] = dateToDays
}

// daysToDate implements `std.daysToDate(days int64) → time.Time`. The
// result is the UTC midnight of `1970-01-01 + days`. Negative inputs map
// to pre-epoch UTC midnights.
func daysToDate(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: std.daysToDate needs 1, got %d", ErrArity, len(args))
	}

	days, ok := toInt64(args[0])
	if !ok {
		return nil, fmt.Errorf("%w: std.daysToDate arg 0: expected int64, got %T", ErrArgType, args[0])
	}

	return time.Unix(days*secondsPerDay, 0).UTC(), nil
}

// dateToDays implements `std.dateToDays(t time.Time) → int64`. The
// result is `floor(t.UTC() / 86400)` in epoch-days. This truncates to
// the UTC day, so values with a sub-day time component yield the same
// answer as their UTC-midnight counterpart.
func dateToDays(args []any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: std.dateToDays needs 1, got %d", ErrArity, len(args))
	}

	when, ok := args[0].(time.Time)
	if !ok {
		return nil, fmt.Errorf("%w: std.dateToDays arg 0: expected time.Time, got %T", ErrArgType, args[0])
	}

	secs := when.UTC().Unix()
	// Go's integer division truncates toward zero; for pre-epoch
	// fractional days this would round toward 1970. Emulate true floor
	// so that `daysToDate(dateToDays(t))` is idempotent for all t.
	days := secs / secondsPerDay
	if secs < 0 && secs%secondsPerDay != 0 {
		days--
	}

	return days, nil
}
