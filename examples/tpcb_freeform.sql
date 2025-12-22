-- ============================================================================
-- TPC-B-LIKE BENCHMARK TEST FOR PGBENCH
-- ============================================================================
-- This script contains the complete TPC-B-like benchmark test structure
-- including schema, data loading, workload, and cleanup sections.
-- ============================================================================

-- ============================================================================
-- SECTION 1: SCHEMA CREATION
-- ============================================================================
-- Creates the four core tables used in the TPC-B-like benchmark

-- Branches table (scale factor determines row count, typically small)
DROP TABLE IF EXISTS pgbench_history CASCADE;
DROP TABLE IF EXISTS pgbench_accounts CASCADE;
DROP TABLE IF EXISTS pgbench_tellers CASCADE;
DROP TABLE IF EXISTS pgbench_branches CASCADE;

CREATE TABLE pgbench_branches (
    bid INTEGER NOT NULL PRIMARY KEY,
    bbalance INTEGER,
    filler CHAR(88)
);

-- Tellers table (10 tellers per branch)
CREATE TABLE pgbench_tellers (
    tid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    tbalance INTEGER,
    filler CHAR(84)
);

-- Accounts table (100,000 accounts per branch - largest table)
CREATE TABLE pgbench_accounts (
    aid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    abalance INTEGER,
    filler CHAR(84)
);

-- History table (append-only, records all transactions)
CREATE TABLE pgbench_history (
    tid INTEGER,
    bid INTEGER,
    aid INTEGER,
    delta INTEGER,
    mtime TIMESTAMP,
    filler CHAR(22)
);

-- Create indexes for performance
CREATE INDEX pgbench_accounts_bid_idx ON pgbench_accounts (bid);
CREATE INDEX pgbench_tellers_bid_idx ON pgbench_tellers (bid);

-- ============================================================================
-- SECTION 2: DATA LOADING
-- ============================================================================
-- Populate tables with initial data
-- Scale factor (s) determines the size: branches=s, tellers=10*s, accounts=100000*s

-- Example for scale factor = 1
-- In practice, pgbench -i handles this initialization

-- Load branches (1 branch per scale factor)
INSERT INTO pgbench_branches (bid, bbalance, filler)
SELECT
    generate_series(1, 1), -- scale factor
    0,
    REPEAT('x', 88);

-- Load tellers (10 tellers per branch)
INSERT INTO pgbench_tellers (tid, bid, tbalance, filler)
SELECT
    generate_series(1, 10), -- 10 * scale factor
    ((generate_series(1, 10) - 1) / 10) + 1,
    0,
    REPEAT('x', 84);

-- Load accounts (100,000 accounts per branch)
INSERT INTO pgbench_accounts (aid, bid, abalance, filler)
SELECT
    generate_series(1, 100000), -- 100000 * scale factor
    ((generate_series(1, 100000) - 1) / 100000) + 1,
    0,
    REPEAT('x', 84);

-- Analyze tables for query optimization
VACUUM ANALYZE pgbench_branches;
VACUUM ANALYZE pgbench_tellers;
VACUUM ANALYZE pgbench_accounts;
VACUUM ANALYZE pgbench_history;

-- ============================================================================
-- SECTION 3: WORKLOAD TRANSACTION
-- ============================================================================
-- The core TPC-B-like transaction that pgbench executes repeatedly
-- Variables used: :aid (account id), :bid (branch id), :tid (teller id), :delta (amount)

-- Transaction begins
BEGIN;

-- Update account balance
UPDATE pgbench_accounts
SET abalance = abalance + :delta
WHERE aid = :aid;

-- Read updated account balance
SELECT abalance
FROM pgbench_accounts
WHERE aid = :aid;

-- Update teller balance
UPDATE pgbench_tellers
SET tbalance = tbalance + :delta
WHERE tid = :tid;

-- Update branch balance (this creates contention in high concurrency)
UPDATE pgbench_branches
SET bbalance = bbalance + :delta
WHERE bid = :bid;

-- Record transaction in history
INSERT INTO pgbench_history (tid, bid, aid, delta, mtime)
VALUES (:tid, :bid, :aid, :delta, CURRENT_TIMESTAMP);

-- Commit transaction
COMMIT;

-- ============================================================================
-- WORKLOAD EXECUTION NOTES
-- ============================================================================
-- In pgbench, variables are randomly selected:
-- - :aid = random(1, naccounts)
-- - :tid = random(1, ntellers)
-- - :bid = random(1, nbranches)
-- - :delta = random(-5000, 5000)
--
-- The bottleneck is typically the pgbench_branches table update due to
-- contention when multiple transactions try to update the same branch.
--
-- To run this workload with pgbench:
-- pgbench -c <clients> -j <threads> -T <duration> -P <progress_interval> <dbname>
-- ============================================================================

-- ============================================================================
-- SECTION 4: CLEANUP
-- ============================================================================
-- Drop all benchmark tables and indexes

DROP TABLE IF EXISTS pgbench_history CASCADE;
DROP TABLE IF EXISTS pgbench_accounts CASCADE;
DROP TABLE IF EXISTS pgbench_tellers CASCADE;
DROP TABLE IF EXISTS pgbench_branches CASCADE;

-- ============================================================================
-- END OF TPC-B-LIKE BENCHMARK SCRIPT
-- ============================================================================
