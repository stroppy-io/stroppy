--+ section cleanup
--= query
DROP TABLE IF EXISTS pgbench_history CASCADE;
--= query
DROP TABLE IF EXISTS pgbench_accounts CASCADE;
--= query
DROP TABLE IF EXISTS pgbench_tellers CASCADE;
--= query
DROP TABLE IF EXISTS pgbench_branches CASCADE;
--= query
DROP FUNCTION IF EXISTS tpcb_transaction;


--+ section create_schema
--= query
CREATE TABLE pgbench_branches (
    bid INTEGER NOT NULL PRIMARY KEY,
    bbalance INTEGER,
    filler CHAR(88)
);

--= query
CREATE TABLE pgbench_tellers (
    tid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    tbalance INTEGER,
    filler CHAR(84)
);

--= query
CREATE TABLE pgbench_accounts (
    aid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    abalance INTEGER,
    filler CHAR(84)
);

--= query
CREATE TABLE pgbench_history (
    tid INTEGER,
    bid INTEGER,
    aid INTEGER,
    delta INTEGER,
    mtime TIMESTAMP,
    filler CHAR(22)
);
--= query
CREATE INDEX pgbench_accounts_bid_idx ON pgbench_accounts (bid);
--= query
CREATE INDEX pgbench_tellers_bid_idx ON pgbench_tellers (bid);

--= query 
CREATE OR REPLACE FUNCTION tpcb_transaction(
    p_aid INTEGER,
    p_tid INTEGER,
    p_bid INTEGER,
    p_delta INTEGER
)
RETURNS INTEGER
LANGUAGE plpgsql
AS $$
DECLARE
    v_balance INTEGER;
BEGIN
    -- Update account balance
    UPDATE pgbench_accounts
    SET abalance = abalance + p_delta
    WHERE pgbench_accounts.aid = p_aid;

    -- Get the updated account balance
    SELECT pgbench_accounts.abalance INTO v_balance
    FROM pgbench_accounts
    WHERE pgbench_accounts.aid = p_aid;

    -- Update teller balance
    UPDATE pgbench_tellers
    SET tbalance = tbalance + p_delta
    WHERE pgbench_tellers.tid = p_tid;

    -- Update branch balance
    UPDATE pgbench_branches
    SET bbalance = bbalance + p_delta
    WHERE pgbench_branches.bid = p_bid;

    -- Insert history record
    INSERT INTO pgbench_history (tid, bid, aid, delta, mtime, filler)
    VALUES (p_tid, p_bid, p_aid, p_delta, CURRENT_TIMESTAMP, 'tpcb_tx');

    RETURN v_balance;
END;
$$;

--+ section analyze
--= query
VACUUM ANALYZE pgbench_branches;
--= query
VACUUM ANALYZE pgbench_tellers;
--= query
VACUUM ANALYZE pgbench_accounts;
--= query
VACUUM ANALYZE pgbench_history;

--+ section insert
--= query
INSERT INTO pgbench_branches (bid, bbalance, filler)
SELECT
    generate_series(1, 1), -- scale factor
    0,
    REPEAT('x', 88);

--= query
INSERT INTO pgbench_tellers (tid, bid, tbalance, filler)
SELECT
    generate_series(1, 10), -- 10 * scale factor
    ((generate_series(1, 10) - 1) / 10) + 1,
    0,
    REPEAT('x', 84);

--= query
INSERT INTO pgbench_accounts (aid, bid, abalance, filler)
SELECT
    generate_series(1, 100000), -- 100000 * scale factor
    ((generate_series(1, 100000) - 1) / 100000) + 1,
    0,
    REPEAT('x', 84);

--+ section workload
--= query
SELECT tpcb_transaction(:p_aid, :p_tid, :p_bid, :p_delta)

