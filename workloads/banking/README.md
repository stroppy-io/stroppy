# banking

Bank-transfer invariant workload: money moves between accounts, and the run fails
if the money or the accounts' tokens stop adding up.

## What it checks

Every transfer moves an amount from one account to another and **rotates their
tokens**. The token is a `UNIQUE` value that swaps between the two rows, so it is
a second witness of the same transfer: a commit that is lost or applied twice
breaks the token set even when the balances happen to sum correctly.

After the workload step, `verify` asserts both invariants:

- `sum(balance)` is exactly `accounts * 1000` — no money created or destroyed;
- the tokens are still the permutation `1..accounts` — none lost, none duplicated.

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
[Macavity](https://github.com/CrystallineCore/Macavity), which hooks the
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
- **`crash` restarts the cluster.** It SIGKILLs the backend; the postmaster then
  terminates *every* other backend and recovers, so other VUs lose their
  connections too and iterations fail while recovery runs (sub-second on a small
  cluster). Point it at a disposable database, and expect the run to report
  failures — the invariants are what must still hold afterwards.
- **The extension must be installed and the role must be allowed to use it.**
  `setup_injection` runs `CREATE EXTENSION IF NOT EXISTS macavity` and fails early
  if that is not possible; Macavity revokes its functions from `PUBLIC`, so the
  connecting role needs superuser or an explicit
  `GRANT EXECUTE ON FUNCTION macavity_arm(text, text, integer)`.
- Build Macavity against the server's headers with `make USE_PGXS=1 with_llvm=no`
  — the plain `make` fails on the LLVM bitcode rule unless `clang-19` is present.
