// Package metrics implements the zero-allocation measurement core of the
// stroppy next engine: per-VU HDR-style histograms, shard storage, a
// background reporter that merges shards on a tick, and a console sink.
//
// # Memory model
//
// The package follows RFC 0001 §6: measurement state is allocated once, in the
// plan phase, and only mutated during the hot phase. A [Registry] assigns an
// integer [MetricHandle] / [CounterHandle] to each instrument at plan time and
// resolves its static tags (step, tx, table, name) there and only there — the
// hot path never touches a tag, a string or a map. Once every instrument is
// registered the registry is frozen ([Registry.Freeze]); [Registry.NewShard]
// then preallocates one [Shard] per VU: a flat slice of histograms plus a flat
// slice of counters. Recording a value is a bucket-index computation followed by
// a single array increment.
//
// # Single writer, concurrent reporter
//
// Each shard has exactly one writer goroutine (the VU it belongs to), so writes
// need no coordination among writers. The one concurrency concern is the
// [Reporter] goroutine, which reads every shard while its writer is running.
//
// The RFC sketches two snapshot strategies (per-shard double-buffer swap, or
// plain writes with torn interval reads). Neither is race-detector clean with a
// concurrently reading reporter: a plain int64 written by the VU and read by the
// reporter is, by definition, a data race that "go test -race" flags. Because a
// clean race detector is a hard exit criterion, this package realises the RFC's
// "plain write / torn read" intent as the minimal synchronization that keeps it:
// histogram buckets and counters are relaxed atomics (sync/atomic.Int64).
//
//   - Hot path: one atomic Add on the bucket (plus a plain running sum and max,
//     see below). Single-writer means the add is always uncontended, so it stays
//     lock-free, zero-allocation and a handful of nanoseconds — benchmarked well
//     under the 10 ns budget. This is the whole cost of choosing atomics over
//     truly-plain writes.
//   - Interval snapshot: the reporter Loads each bucket atomically. Loads across
//     a histogram are not one instant, so an interval view can be slightly torn —
//     acceptable, because buckets only ever grow, so every torn read is a valid
//     intermediate cumulative state.
//   - Exact final report: [Reporter.Stop] joins the tick goroutine and is called
//     only after all writers have stopped (established by the caller, e.g. a
//     WaitGroup). With no concurrent writer, the final aggregation is exact.
//
// The exact running sum and max are kept as plain int64 fields (not atomics):
// they are read only by the final, post-stop summary, never during a live tick,
// so no race exists and the hot path avoids a compare-and-swap for max and a
// second atomic for sum. Interval percentiles and counts are derived purely from
// the atomic buckets.
//
// # Histogram precision
//
// See [Histogram] for the exact HDR bucketing math and the documented relative
// precision (better than 1%).
package metrics
