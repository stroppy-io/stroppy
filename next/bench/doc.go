// Package bench is the load-generation heart of the stroppy next engine: the
// per-worker VU runtime and the executor policies that drive a [Handler] under
// the four load shapes of RFC 0001 §7.2 (Once, Pool, Closed, Open).
//
// The executor policies are the M3 slice; the M6a slice adds the runnable
// surface on top of them: [Test]/[StepDef]/[Main] wire the executors into a
// dag.Graph (each [Executor.Run] has the exact func(ctx) error shape dag.Node.Run
// expects), parse options and driver slots from the environment, and drive a
// single run-level reporter over one shared registry spanning every step.
//
// # Run wiring (M6a)
//
// A [Test] is declarative data: a name, seed, options struct, driver slots and a
// step DAG. [Main] parses env/flags into it, builds one [Executor] per step over
// a shared [metrics.Registry], materializes them all (which freezes the
// registry), then runs the graph under one [metrics.Reporter] so a single exact
// summary covers the whole run. Each step's rng streams derive from a stable step
// id (the FNV-1a hash of its name; see stepID's stability contract) so a run is
// reproducible and independent of step ordering.
//
// Each VU pins a [driver.Conn] to its step's default slot on first use in Init
// ([VU.ConnE], returning the connect failure as a first-class error; [VU.Conn]
// is the panic-on-failure form for trivial FuncOnce bodies) and prepares SQL
// handles per query ([VU.PrepareE]/[VU.Prepare]). Both are plan-phase work:
// establishing a connection or statement for the first time inside a hot-loop
// Iter is an error, since connecting is not hot-path work. Connections are
// reconnected per step (closed after the step's Close) — an accepted PoC cost,
// to revisit post-PoC.
// Step-level [StepDef.Retry] maps to the executor retry around a single Iter, not
// dag-level node retry (re-running a whole load step is not a PoC need).
//
// # Allocation phases (RFC 0001 §6)
//
// Construction is the plan phase: an executor constructor allocates every VU,
// its per-VU metrics [metrics.Shard], its [mem.Arena], its rng-stream cache and
// the built-in instruments. The hot phase — the per-iteration body — touches
// only that preallocated state and allocates nothing in harness code:
//
//   - The VU's [mem.Arena] is Reset at the start of every Iter, so variable-size
//     data reuses the same chunks batch after batch.
//   - [VU.Rand] memoizes derived streams, so repeated calls for the same stream
//     id after warm-up are a map read, not a Derive.
//   - Metric recording is a bucket-index increment into the VU's private shard.
//
// The steady-state zero-alloc property is gated two ways: an AllocsPerRun gate
// on the record helpers, and a ReadMemStats Mallocs-delta measurement around a
// running Closed executor (see closed_test.go for the method).
//
// # Determinism (RFC 0001 §5)
//
// Run-repro is a non-goal at the concurrent layer: worker scheduling is not
// bit-reproducible across runs, and the engine does not aspire to bit-identical
// interleaving. The contract is data-repro — the generated dataset is
// bit-identical given (seed, scale) — plus deterministic per-step rng streams.
// Every iteration is keyed by a uint64 cycle; the cycle selects both the rng
// draws ([VU.Rand] streams derive from run seed + step id + stream id, seeked by
// cycle) and, for Pool, the assigned item. Cycle allocation is deterministic and
// contention-free:
//
//   - Closed: each VU owns a contiguous cycle range; VU k walks base_k,
//     base_k+1, ... where base_k = k * (2^64 / vus). Ranges never overlap, so
//     the cycle->VU assignment is a pure function of vus.
//   - Pool: cycle == item index, independent of which worker steals the item, so
//     item i always draws from the same rng position.
//   - Open: cycle == schedule index; VU k serves indices k, k+vus, k+2vus, ...
//     The partition is a pure function of index, independent of wall-clock
//     timing, so a paced run is exactly reproducible.
//
// # Open-loop saturation model (coordinated-omission-aware)
//
// Open precomputes an arrival schedule: N = floor(rate * duration) slots at
// scheduledStart(i) = runStart + i/rate seconds. Each VU sleeps (one reused
// timer, no busy-spin) until its next slot, then runs one Iter. It records two
// measurements per iteration:
//
//   - servicetime: the Iter duration.
//   - waittime: actualStart - scheduledStart, clamped at zero — the schedule lag
//     that NoSQLBench/HdrHistogram call the coordinated-omission correction.
//
// When a VU is saturated (an Iter runs longer than the VU's slot spacing,
// interval*vus), slots are NOT dropped: the VU runs them late, back to back,
// with monotonically growing waittime (a queued/lagging model, not a
// rate-degrading one). Each late iteration — one that finishes past its own next
// scheduled slot — increments the behind-schedule gauge ([Executor.BehindSchedule]),
// which stays zero on a healthy paced run and grows under saturation. This is
// the difference between measuring the database and measuring the harness's
// politeness (RFC 0001 §7.2).
//
// # Error modes (ported from v5)
//
// v5's ErrorModeName (silent|log|throw|fail|abort, internal/static/helpers.ts)
// maps onto the Go [ErrorMode] enum. The Go model has one fewer value because Go
// returns errors as values rather than throwing:
//
//	v5 "silent" -> Silent : count the error, no log, keep running.
//	v5 "log"    -> Log    : count + log to stderr, keep running (default).
//	v5 "throw"  -> Fail  \ In v5 "throw" rethrows the error into the surrounding
//	v5 "fail"   -> Fail  / control flow while "fail" marks the run failed; both
//	                       let the run continue and end up failed. In Go the Iter
//	                       error is already a value surfaced to the executor loop,
//	                       so the two collapse: count + log, keep running, and
//	                       Run returns the first error as an aggregate at the end.
//	v5 "abort"  -> Abort  : count + log, cancel the executor context (in-flight
//	                        Iters finish and Close runs), Run returns promptly.
//
// throw is merged into Fail rather than Abort because in v5 an uncaught throw
// does not stop the run — only abort does — so Fail (finish-but-failed) is the
// faithful home for it.
//
// A [RetryPolicy] wraps each Iter: retryable errors are retried up to
// MaxAttempts with an optional backoff before the [ErrorMode] classification
// sees them. A retried-then-succeeded iteration surfaces no error and is not
// counted in the error counter; each retry bumps the retries counter.
package bench
