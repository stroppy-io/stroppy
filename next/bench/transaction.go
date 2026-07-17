package bench

import (
	"context"
	"math/rand"
	"time"

	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/metrics"
)

// RetryOpts bounds a retry loop. MaxAttempts is the total attempt count (the
// first attempt plus retries); a value below 1 is treated as 1, so a zero-value
// RetryOpts runs the call exactly once with no replay — retry is opt-in and
// disabled by default. Backoff is the sleep between retries; Jitter is added
// uniformly on top of it ([0,Jitter)). Retries are driven by a classifier —
// only the dbdrv knows which errors are transient (D2/D10).
type RetryOpts struct {
	MaxAttempts int
	Backoff     time.Duration
	Jitter      time.Duration
}

// Retry calls fn up to opts.MaxAttempts times, replaying it when classify maps
// the returned error to [driver.Retry], sleeping opts.Backoff (+ jitter)
// between attempts. It is the retry primitive a transaction ([Transaction]) and
// a plain-query path share: fn is the unit of replay, classify decides what is
// transient. A nil classifier disables retry (every error is terminal); a
// zero-value RetryOpts runs fn once. The steady-state success path returns nil
// and never consults classify, so the enum only exists on the failure branch.
//
// Jitter uses the process rand source, not the step's seed-derived stream:
// retry sleep timing is not part of the data-repro contract (only rng draws
// are), and the retry path is rare enough that its timing does not feed back
// into measured steady-state results.
func Retry(
	ctx context.Context,
	classify func(error) driver.Action,
	opts RetryOpts,
	fn func() error,
) error {
	if opts.MaxAttempts < 1 {
		opts.MaxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if classify == nil || classify(err) != driver.Retry || attempt == opts.MaxAttempts {
			return err
		}
		sleepBackoff(ctx, opts.Backoff, opts.Jitter)
	}
	return lastErr
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
// to opts.MaxAttempts attempts with opts.Backoff (+ jitter) between them. A
// transaction replays whole, not per-query — the unit the TPC-C new_order
// semantics require (a serialization retry re-runs every statement against
// fresh state).
//
// It is built on [Retry]: the retry decision and backoff/jitter are shared with
// the plain-query path; the closure owns the tx lifecycle (Begin/Commit/
// Rollback) and telemetry. classify is the connection's own driver classifier
// (a [driver.Driver.Classify] bound to the connection's backend); pass nil to
// disable retry and treat every error as terminal. rec, when non-nil, records
// whole-tx wall-clock latency (Begin -> terminal) into rec.Latency on a commit
// and tallies outcome counts on every terminal outcome. Returns nil on a
// committed tx, the terminal fn/commit error otherwise.
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
	record := rec != nil && rec.Shard != nil
	var start time.Time
	if record {
		start = time.Now()
	}

	attempt := 0
	err := Retry(ctx, classify, opts, func() error {
		attempt++
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
				if record {
					rec.Shard.Record(rec.Latency, time.Since(start).Nanoseconds())
				}
			}
			return nil
		}

		if rec != nil {
			rec.RolledBack++
		}
		return terminal
	})
	// Retried counts replays across the whole call (success after retries or
	// exhaustion), tallied once from the final attempt number.
	if rec != nil && attempt > 1 {
		rec.Retried += int64(attempt - 1)
	}
	return err
}

// sleepBackoff sleeps for backoff plus uniform jitter in [0,jitter), returning
// early if ctx is canceled. The retry path is rare (only serialization-class
// failures), so the timer allocation here is off the steady-state hot path.
func sleepBackoff(ctx context.Context, backoff, jitter time.Duration) {
	d := backoff
	if jitter > 0 {
		d += time.Duration(rand.Int63n(int64(jitter)))
	}
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
