package bench

import (
	"context"
	"time"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/metrics"
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

// TxRecorder is the per-VU telemetry seam a [Transaction] records into: whole-tx
// wall-clock latency (Begin -> terminal Commit/Rollback, including any replay
// attempts) plus outcome counts. It replaces D9's count-only *TxStats seam:
// D6's unified telemetry routes the latency through an author-declared histogram
// (typically per-tx via [Def.Histogram] with [Tag] values), and the counts stay
// for callers that want the helper's own outcome accounting.
//
// Recording is gated by Shard: a nil Shard (or a nil *TxRecorder) records
// nothing — for an author who wants raw uninstrumented timing. A non-nil Shard
// records whole-tx wall-clock latency (Begin -> terminal) into Latency on a
// commit; any handle value is valid, including 0 (the first-registered
// histogram). The author typically stores one TxRecorder per VU in step state,
// points Latency at the picked tx's histogram handle per iteration, and leaves
// Shard fixed across iters.
type TxRecorder struct {
	// Latency is the histogram handle that records whole-tx wall-clock latency.
	// Any value is valid (handle 0 is the first-registered histogram); it is
	// read only when Shard is non-nil.
	Latency metrics.MetricHandle
	// Shard is the per-VU shard Latency records into. It is the recording
	// switch: nil disables latency recording (outcome counters still accrue).
	Shard *metrics.Shard

	// Outcome counters, valid only when the recorder is non-nil at the call.
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
// error as terminal. rec, when non-nil, records whole-tx wall-clock latency
// (Begin -> terminal) into rec.Latency on a commit and tallies outcome counts
// on every terminal outcome. Returns nil on a committed tx, the terminal
// fn/commit error otherwise (including errRollback-style sentinels the caller
// may reinterpret).
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
	rec *TxRecorder,
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

	// Whole-tx latency = wall clock from the first Begin to the terminal
	// Commit/Rollback, so a multi-attempt tx reports the full retry cost. The
	// start is captured once, outside the attempt loop; only the rare recording
	// path (commit success with a non-nil Shard) calls time.Since. Shard is the
	// gate (handle 0 is a valid histogram, so Latency cannot signal on/off).
	record := rec != nil && rec.Shard != nil
	var start time.Time
	if record {
		start = time.Now()
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
			if rec != nil {
				rec.Committed++
				if attempt > 1 {
					rec.Retried += int64(attempt - 1)
				}
				if record {
					rec.Shard.Record(rec.Latency, time.Since(start).Nanoseconds())
				}
			}
			return nil
		}

		lastErr = terminal
		if !retryable(terminal) || attempt == opts.MaxAttempts {
			if rec != nil {
				rec.RolledBack++
				if attempt > 1 {
					rec.Retried += int64(attempt - 1)
				}
			}
			return terminal
		}

		if rec != nil {
			rec.RolledBack++
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
