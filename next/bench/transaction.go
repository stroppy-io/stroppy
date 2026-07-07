package bench

import (
	"context"
	"time"

	"github.com/stroppy-io/stroppy/next/driver"
)

// RetryOpts bounds a [Transaction] retry loop. MaxAttempts is the total attempt
// count (the first attempt plus retries); a value below 1 is treated as 1, so a
// zero-value RetryOpts runs the tx exactly once with no replay. Backoff is the
// sleep between retries; zero sleeps nothing. Retries are driven by the
// connection's own driver classifier — only the dbdrv knows which errors are
// transient (D2/D10).
type RetryOpts struct {
	MaxAttempts int
	Backoff     time.Duration
}

// TxStats tallies transaction outcomes for a caller that wants the helper's own
// accounting — the seam D6's unified telemetry plugs into once it lands. Fields
// are int64 so a *TxStats can be owned by one VU and aggregated at Close without
// synchronization, matching the per-VU shard pattern. A nil *TxStats is accepted
// by [Transaction] to skip recording: a caller with richer per-VU counts (tpcc's
// txCounts) passes nil to avoid double-counting.
type TxStats struct {
	Committed  int64
	RolledBack int64
	Retried    int64
}

// Transaction runs fn inside a transaction on conn at iso. It commits when fn
// returns nil, rolls back when fn returns a non-nil error, and replays the whole
// fn when classify maps the fn error or a commit failure to [driver.Retry], up
// to opts.MaxAttempts attempts with opts.Backoff between them. A transaction
// replays whole, not per-query — the unit the TPC-C new_order semantics require
// (a serialization retry re-runs every statement against fresh state).
//
// classify is the connection's own driver classifier (a [driver.Driver.Classify]
// bound to the connection's backend); pass nil to disable retry and treat every
// error as terminal. stats records committed/rolled-back/retried counts when
// non-nil. Returns nil on a committed tx, the terminal fn/commit error
// otherwise (including errRollback-style sentinels the caller may reinterpret).
//
// Begin failures are not retried here: a connect-level fault is surfaced
// immediately for the executor's ErrorMode to handle (it is rarely transient in
// the serialization sense a tx retry addresses).
func Transaction(
	ctx context.Context,
	conn driver.Conn,
	classify func(error) driver.Action,
	iso driver.Isolation,
	opts RetryOpts,
	stats *TxStats,
	fn func(tx driver.Tx) error,
) error {
	if opts.MaxAttempts < 1 {
		opts.MaxAttempts = 1
	}
	retryable := func(err error) bool {
		if classify == nil || err == nil {
			return false
		}
		return classify(err) == driver.Retry
	}

	var lastErr error
	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		tx, err := conn.Begin(ctx, iso)
		if err != nil {
			return err
		}

		fnErr := fn(tx)
		var terminal error
		if fnErr == nil {
			terminal = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
			terminal = fnErr
		}

		if terminal == nil {
			if stats != nil {
				stats.Committed++
				if attempt > 1 {
					stats.Retried += int64(attempt - 1)
				}
			}
			return nil
		}

		lastErr = terminal
		if !retryable(terminal) || attempt == opts.MaxAttempts {
			if stats != nil {
				stats.RolledBack++
				if attempt > 1 {
					stats.Retried += int64(attempt - 1)
				}
			}
			return terminal
		}

		if stats != nil {
			stats.RolledBack++
		}
		sleepBackoff(ctx, opts.Backoff)
	}
	return lastErr
}

// sleepBackoff sleeps for d, returning early if ctx is canceled. The retry path
// is rare (only serialization-class failures), so the timer allocation here is
// off the steady-state hot path.
func sleepBackoff(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
