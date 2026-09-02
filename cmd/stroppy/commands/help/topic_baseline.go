package help

func init() {
	Register(Topic{
		Name:  "baseline",
		Short: "Machine baseline: stroppy's own performance ceiling, no database needed",
		Long: `BASELINE

  stroppy baseline measures stroppy itself on the current machine. It needs no
  database: the result is the ceiling a real run on this machine can never
  exceed, and the reference point for judging whether real-run numbers are
  sane.

  Run it:
    - after installing a new stroppy build, to sanity-check the machine;
    - when a real database run looks slow, to see whether stroppy or the
      database is the bottleneck;
    - before/after infrastructure changes (VM resize, kernel, CPU governor).

TIERS

  noop   The built-in baseline workload against the noop driver: pure
         framework cost. Row generation, argument handling, transaction
         bookkeeping. No network, no protocol.
  wire   The same workload through the postgres driver against a pg-noop
         blackhole server on loopback: framework + pgx pool + PostgreSQL
         wire protocol + TCP. pg-noop (github.com/stroppy-io/pg-noop)
         discards all I/O faster than stroppy can produce it, so this tier
         measures the client side only.

  Each tier reports:
    load      rows/s through the typed insert path (noop: generation drain;
              wire: native COPY, which pg-noop drains)
    tx 1 VU   single-core transaction throughput and average latency
    tx N VU   parallel throughput and latency at GOMAXPROCS virtual users

READING THE NUMBERS

  Compare a real database run against the wire tier:
    - real run near the wire ceiling  -> client-bound; raise VUs, pool size,
      or bulk size, or reduce stroppy-side work before tuning the database;
    - real run far below the ceiling    -> the database or the network is the
      bottleneck.

  The gap between the noop and wire tiers is the protocol + loopback cost of
  the postgres path on this machine.

VERDICTS

  Verdicts check hardware-independent invariants, not absolute thresholds, so
  they hold on a Raspberry Pi and on a 128-core server alike:

    vu scaling          parallel throughput vs the single-VU rate times VUs.
                        Low scaling means contention. Note: stroppy's metric
                        pipeline itself contends at very high iteration rates,
                        so a scaling warning on a fast machine at high VUs can
                        be stroppy's own overhead, not a broken machine.
    loopback latency    wire p99 should stay well under 1ms on loopback.
                        Higher points at VM steal, power-saving states, or a
                        saturated host.
    latency noise       p99/p50 spread on the wire tier; large spread means a
                        noisy neighbor or scheduling interference.
    errors              any failed iteration taints the numbers.
    tier ordering       the noop tier must stay ahead of the wire tier;
                        otherwise the measurement itself is suspect.

  Absolute magnitudes vary wildly by hardware; when in doubt, rerun and
  compare against this machine's own history instead of guessing.

HISTORY

  Every run saves a versioned JSON report under ~/.stroppy/baselines/
  (suppress with --no-save). The text output shows the delta versus the
  previous saved run. --json prints the full report for tooling.

SERVER BINARY

  The wire tier needs the pg-noop binary, resolved in order:
    1. embedded copy (release builds carrying the server binary)
    2. cache: ~/.stroppy/bin/pg-noop/<version>/pgnoop
    3. download from the pinned github.com/stroppy-io/pg-noop release,
       verified against a digest compiled into stroppy itself (a tampered
       release cannot pass); interactive consent is required on a terminal
       (--download always|never controls this)
  --server-path PATH or STROPPY_PG_NOOP_PATH supply a binary directly, which
  also serves air-gapped machines and CI.

FLAGS

  --quick              shorter phases, smaller load (~5s)
  --json               print the report as JSON
  --tiers noop,wire    select a subset of tiers
  --vus N              VU count for the parallel phase (default GOMAXPROCS)
  --duration D         phase duration (default 3s, 1s with --quick)
  --rows N             load rows (default 250000, 100000 with --quick)
  --server-path PATH   explicit pg-noop binary
  --download MODE      ask (default), always, or never
  --no-save            do not write the history report

SEE ALSO

  stroppy run baseline --help     run the workload manually against any driver
  stroppy probe                   list registered workloads
`,
	})
}
