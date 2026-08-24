package tpcc

import (
	"errors"
	"testing"
)

const (
	cc1Name = "CC1 sum(W_YTD) = sum(D_YTD)"
	cc4Name = "CC4 sum(O_OL_CNT) = count(order_line)"
)

// runConsistency invokes checkConsistency with empty district/order aggregates
// (so only CC1/CC4 fire) and returns the names of passed and failed checks.
func runConsistency(
	t *testing.T,
	cc1WSum, cc1DSum float64, cc1WErr, cc1DErr error,
	cc4OSum, cc4OlCnt int64, cc4OErr, cc4OlErr error,
) (passed, failed []string) {
	t.Helper()

	check := func(name string, ok bool) {
		if ok {
			passed = append(passed, name)
		} else {
			failed = append(failed, name)
		}
	}

	checkConsistency(check,
		map[string]int64{}, map[string]int64{}, map[string]noStat{},
		cc1WSum, cc1DSum, cc1WErr, cc1DErr,
		cc4OSum, cc4OlCnt, cc4OErr, cc4OlErr,
	)

	return passed, failed
}

func has(list []string, want string) bool {
	for _, got := range list {
		if got == want {
			return true
		}
	}

	return false
}

func TestCheckConsistencyCC1QueryErrorFailsCheck(t *testing.T) {
	cases := []struct {
		name       string
		werr, derr error
	}{
		{"warehouse sum error", errors.New("boom"), nil},
		{"district sum error", nil, errors.New("boom")},
		{"both sums error", errors.New("boom"), errors.New("boom")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Equal (zero) sums that would spuriously pass without the error guard.
			_, failed := runConsistency(t, 0, 0, tc.werr, tc.derr, 0, 0, nil, nil)
			if !has(failed, cc1Name) {
				t.Fatalf("CC1 did not fail on query error; failed=%v", failed)
			}
		})
	}
}

func TestCheckConsistencyCC4QueryErrorFailsCheck(t *testing.T) {
	cases := []struct {
		name        string
		oerr, olErr error
	}{
		{"orders sum error", errors.New("boom"), nil},
		{"order_line count error", nil, errors.New("boom")},
		{"both error", errors.New("boom"), errors.New("boom")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Equal (zero) counts that would spuriously pass without the error guard.
			_, failed := runConsistency(t, 0, 0, nil, nil, 0, 0, tc.oerr, tc.olErr)
			if !has(failed, cc4Name) {
				t.Fatalf("CC4 did not fail on query error; failed=%v", failed)
			}
		})
	}
}

func TestCheckConsistencyAggregatesStillComparedWhenNoError(t *testing.T) {
	t.Run("equal sums pass", func(t *testing.T) {
		passed, failed := runConsistency(t, 100, 100, nil, nil, 42, 42, nil, nil)
		if !has(passed, cc1Name) || !has(passed, cc4Name) {
			t.Fatalf("equal sums should pass CC1/CC4; passed=%v failed=%v", passed, failed)
		}
	})

	t.Run("unequal sums fail", func(t *testing.T) {
		_, failed := runConsistency(t, 100, 200, nil, nil, 42, 43, nil, nil)
		if !has(failed, cc1Name) || !has(failed, cc4Name) {
			t.Fatalf("unequal sums should fail CC1/CC4; failed=%v", failed)
		}
	})
}
