# banking

Bank-transfer invariant workload: money moves between accounts, and the run fails
if the money or the accounts' tokens stop adding up.

## What it checks

Every transfer moves an amount from one account to another and **rotates their
tokens**. The token is a `UNIQUE` value that swaps between the two rows, so it is
a second witness of the same transfer: a half-applied swap — a row left in the
sentinel, or two rows holding one token — is a state the balances alone cannot show.

After the workload step, `verify` asserts both invariants:

- `sum(balance)` is exactly `accounts * 1000` — no money created or destroyed;
- the tokens are still the permutation `1..accounts` — none lost, none duplicated.

On Picodata the second is not checked, and the run says so: a unique constraint on the
token is not expressible there (unique indexes are limited to sharding-key columns) and
a two-row transfer cannot be atomic (transactions are single-bucket), so that dialect
carries a reduced asset and checks the total balance only. The run's report records it,
under `token_witness`, naming what is therefore not detectable.

`accounts` defaults to 100, so the expected total is 100000.

## The transfer

Read both rows, then swap in three conditional updates rather than two:

```sql
UPDATE bank_account SET token = :token WHERE id = :id AND token = :expected
```

A token moves *conditionally* on the value that was read, so a concurrent
transfer on either row is detected as a conflict — and *retried*, because the
workload widens the retry policy to cover it (the default policy fails closed on
error kinds it does not know, and a conflict detected by the workload itself is
one of those). Retrying is safe because the transaction is the unit of work, and
a repeated transfer cannot break either invariant: it moves the same amount
between the same two accounts and conserves the total. Three updates rather than two because the
swap needs a sentinel: the first row puts its token aside (`-id`), which is
outside `1..accounts` and unique to that row, because two rows cannot hold a token
at once under an immediate `UNIQUE` constraint. OrioleDB's version of this test
could swap in two steps only because it used a deferrable constraint; this keeps
the same semantics on any dialect.

## Parameters

| Flag | Default | Meaning |
|---|---|---|
| `--accounts` | `100` | Account count; the invariant is `accounts * 1000`. |
| `--retry-attempts` | `3` | Transaction attempts before a transfer is a failure. |
| `--tx-isolation` | driver default | `read_committed` (pg/mysql), `serializable` (ydb), `none` (picodata). |
| `--sql-file` | embedded | SQL dialect file override, e.g. a local `pg.sql`. |

## Run it

```bash
stroppy run banking -d pg -D url=postgres://user@localhost:5432/scratch \
  --executor constant-vus --vus 8 --duration 30s
```

Against a disposable database: the workload creates and drops `bank_account`.

## Origin

Derived from the bank-account crash test in OrioleDB
(`test/t/crash/rr_stress_test.py`), which drives the same invariants into engine
crashes through patched stop events. This workload keeps the invariants, the
two-row transfer and the token witness, and drops the engine: it runs on stock
PostgreSQL, and the fault-injection variant (`banking/inject`) uses PostgreSQL's
own injection points instead of a patched server.

## Fault injection (`banking/inject`)

PostgreSQL's own injection points cannot interrupt a commit on a stock build:
they are compiled out, and even a server built with `--enable-injection-points`
has no point in the commit path at all (`xact.c` has none; the one reliable core
point is mid-`UPDATE`). This variant therefore injects through
[Macavity](https://github.com/stroppy-io/Macavity) — the stroppy-io fork of it, pinned
at the commit this repository audited — which hooks the
transaction callbacks through supported APIs.

```bash
stroppy run banking/inject -d pg -D url=postgres://user@localhost:5432/scratch \
  --executor constant-vus --vus 4 --duration 30s \
  --inject-point before_commit --inject-action error --inject-every 25
```

| Flag | Default | Meaning |
|---|---|---|
| `--inject-point` | `before_commit` | Macavity fault point: `before_commit`, `before_abort`, `executor_start`, `executor_end`. |
| `--inject-action` | `error` | `error` fails the commit; `crash` kills the backend. |
| `--inject-occurrence` | `1` | Which matching hit fires, counted from the arming transaction. |
| `--inject-every` | `25` | Arm the injector every Nth transfer; `0` disables injection. |

What to expect:

- **`error` is the safe one, and the workload retries it.** The error arrives at
  `XACT_EVENT_PRE_COMMIT`, before the commit record is written, so the transaction
  is aborted — measured by arming it and watching a counter: the interrupted
  attempt leaves it at 0 and the retry commits it once. The session is left idle,
  not in a failed transaction block. The transfer retries this because the
  injector's error classifies as *unknown*, and the default policy fails closed on
  unknown kinds; the workload widens that map, which is also what makes its own
  lost-rotation conflict retryable.
- **`crash` reinitializes the whole cluster, and the run cannot finish while it is
  recovering.** It SIGKILLs the backend; the postmaster then terminates every other
  backend and reinitializes — same process, no restart — so the pool loses its
  connections and iterates against `FATAL: the database system is in recovery mode`.
  Measured on a durable server — a 32-core host with PostgreSQL 17 — a 10 s run at
  `--inject-every 5` attempted **10156** iterations, **10146 of them failed**, only
  **10** committed, and the run **exits non-zero**. So this action demonstrates that
  the invariants survive crashes; it is not a test that passes. They did survive:
  50 rows, sum 50000, and the token permutation intact, read from the server after the
  run. Point it at a disposable database.
- **`verify` waits for a server that is not answering yet.** It retries a bounded
  number of times (6 s), because otherwise a crashed run could never report on the
  invariants at all — the first refusal used to abort the step. A genuine violation
  is still reported, after the wait and unchanged.
- **The extension needs superuser, and the role running the workload needs the arm
  grant.** Macavity's control file is `trusted = false`, so no `GRANT` substitutes
  for superuser when creating it; a non-superuser role additionally needs
  `GRANT EXECUTE ON FUNCTION macavity_arm(text, text, integer)`. Both were needed in
  practice: with them a non-superuser run armed 689 faults 1:1 and held its
  invariants.
  Where the failure lands depends on whether the extension already exists:
  `setup_injection` fails early only when it is *absent*. Once a superuser has
  created it, `CREATE EXTENSION IF NOT EXISTS` succeeds silently and a missing arm
  grant surfaces mid-run, as a transfer failure — and the run still exits 0, since
  permission errors are ordinary per-iteration errors. Expect a permission error to
  be retried a few times before it appears: it classifies as *unknown*, exactly like
  the injector's own error.
- **Build it into the server it will run in**, from the fork at the audited commit:
  `make macavity-install` clones `stroppy-io/Macavity` at `865b9b42b24c`, builds it with
  `USE_PGXS=1 with_llvm=no` and installs it. It is never fetched prebuilt — the extension
  is a `.so` that has to match the server's ABI. `make macavity` builds without installing.
  The `with_llvm=no` matters: the server's bitcode rule wants clang-19 on Ubuntu 24.04,
  where the packaged headers pull a different LLVM. If the headers are missing there,
  `apt-get download postgresql-server-dev-<version>` + `dpkg-deb -x` avoids that entirely.
  `setup_injection` then creates the extension; a missing one fails the run immediately,
  and CI runs this variant from the same target, so the dependency is exercised there
  rather than assumed.
