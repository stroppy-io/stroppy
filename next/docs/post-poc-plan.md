# Post-PoC plan — stroppy next

PoC (M0–M9) is complete: see [poc-plan.md](poc-plan.md) and [verdict.md](verdict.md).
This plan sequences the next work sessions. Toolchain auto-download (verdict item) is
deliberately **deferred** — not scheduled here.

**Standing invariant — universal driver (N+M).** Preserved from v5: a test is written
once; drivers are interchangeable via per-dialect SQL files with an identical section
layout (`pg.sql` / `mysql.sql` / …). Cost of coverage is N tests + M drivers, never N×M
test variants. Every API decision below is checked against this. Known gap going in:
`next` currently has no dialect-selection mechanism (tpcc embeds `pg.sql` only and pins
`Kind: "pg"`) — designed in S2, exercised in S3.

**Infra.** No local DB. Integration databases run as docker containers on the large VM
(`ssh large-vm`); start pg/mysql containers there as needed. Driver integration tests
key off `STROPPY_TEST_PG_URL` / `STROPPY_TEST_MYSQL_URL` and skip when unset; for local
runs, tunnel (`ssh -L`) or run the suite on the VM.

---

## S1 — Reporting parity + SDK cleanup

The two backlog items with proven demand (verdict gap #1 + the tpcc leakage audit).

**1a. Handler-reachable instruments.** User instruments must be registrable before
`metrics.Registry.Freeze` but usable from handlers. Design options: a `Describe`-style
hook on Handler (called pre-freeze), or instrument declaration on StepDef; pick the one
that keeps hot-path recording a plain `vu.M(handle, v)`. Deliverable: tpcc reports
per-transaction-type latency histograms (p50/p95/p99 per new_order/payment/…), the
per-VU count workaround and hand-rolled `report` step die.

**1b. SDK extractions from the tpcc leakage audit:**
- `bench.Loader` (or equivalent): the fill-batch-flush COPY loop (cols, gen fn, batch
  size) — removes ~110 LOC of generic plumbing from every relational test.
- Pool items: typed or first-class cycle ranges (`PoolCycles(workers, total)`) — kills
  the `"start:end"` string encode/decode.
- `bench.ExecSection(qs)`: run-once prepare+exec of a SQL section (both tests duplicate it).
- `driver.ParseIsolation(name)`: the string→Isolation table out of test code.

Migrate tests/simple + tests/tpcc; all existing gates stay green (race, allocgate,
acceptance on a VM pg container, reproducibility).

Exit: per-tx histograms in tpcc summary; tpcc non-test LOC drops ≥150; no framework
plumbing left in test files beyond domain code.

## S2 — Script API hand-review (interactive, user-driven)

Part-by-part walkthrough of the frozen surface with the user; breaking changes allowed
(pre-public). Agenda per part: current shape → v5 coverage → gaps/limitations → verdict
(keep / change / add).

Parts: Test/options/DriverSlot · StepDef+DAG semantics · Handler/VU lifecycle · driver
surface (Conn/Stmt/Args/Tx/Rows) · sqlfile · metrics/instruments · determinism contract ·
CLI flags.

Prepared input: a **v5 coverage matrix** built from `helpers.ts`/`datagen.ts`/AGENTS.md —
every v5 capability (ENV, Step, DriverX/TxX incl. queryCursor/queryValue, retry helpers,
insertSpec, multi-driver `-d1`, procs variant, error modes, isolation incl. pico rules,
`--steps`, dialect SQL selection, probe metadata) marked covered / consciously dropped /
missing.

Design item owned by this session: the **dialect mechanism** (slot kind → embedded
`<kind>.sql` with the v5 identical-section-layout contract; what happens when a dialect
lacks a section, cf. v5 procs pg/mysql-only rule).

Exit: written findings + change list; changes applied before S3 starts.

## S3 — mysql driver (N+M proof)

- `driver/mysql`: go-sql-driver/mysql (sqldriver-class); Question placeholders (sqlfile
  already emits them); bulk insert via multi-row INSERT (no COPY); isolation mapping;
  port v5's rows normalization lesson (`[]byte`→string for CHAR/VARCHAR scans,
  pkg/driver/sqldriver/rows.go).
- mysql container on large-vm; integration tests behind `STROPPY_TEST_MYSQL_URL`.
- tpcc on mysql: reuse `workloads/tpcc/mysql.sql` verbatim through the S2 dialect
  mechanism.

Exit: tpcc green on mysql **with zero changes to tpcc Go code** — only the SQL file and
the slot kind differ. That result is the N+M proof and a verdict-doc addendum.

## S4 — tpcb + tpch ports

- tpcb first (small; immediate reuse of Loader/ExecSection; pg + mysql dialects from
  workloads/tpcb/).
- tpch: reuse `third_party/gotpc/dbgen` generators (byte-equal port already in-tree),
  SCALE_FACTOR option, q1–q22 execution from workloads/tpch/pg.sql, SF=1 answer
  validation against v5's answers files.
- Same gates as tpcc: determinism (worker-count-invariant load), alloc gates on hot
  paths, validation steps.

Exit: tpcb green on pg+mysql; tpch green on pg with SF=1 answers passing; per-query
metrics for tpch (S1 instruments).

---

Deferred (unscheduled): toolchain auto-download · cloud protocol · results store/OTel ·
ydb/picodata/csv drivers · tpcds port · distributed runs · sandboxing/declarative tiers.
