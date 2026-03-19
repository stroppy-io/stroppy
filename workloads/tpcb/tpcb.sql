--+ cleanup
--=
DROP TABLE IF EXISTS pgbench_history CASCADE;
--=
DROP TABLE IF EXISTS pgbench_accounts CASCADE;
--=
DROP TABLE IF EXISTS pgbench_tellers CASCADE;
--=
DROP TABLE IF EXISTS pgbench_branches CASCADE;
--=
DROP FUNCTION IF EXISTS tpcb_transaction;


--+ create_schema
--=
CREATE TABLE pgbench_branches (
    bid INTEGER NOT NULL PRIMARY KEY,
    bbalance INTEGER,
    filler CHAR(88)
);

--=
CREATE TABLE pgbench_tellers (
    tid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    tbalance INTEGER,
    filler CHAR(84)
);

--=
CREATE TABLE pgbench_accounts (
    aid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    abalance INTEGER,
    filler CHAR(84)
);

--=
CREATE TABLE pgbench_history (
    tid INTEGER,
    bid INTEGER,
    aid INTEGER,
    delta INTEGER,
    mtime TIMESTAMP,
    filler CHAR(22)
);
--=
CREATE INDEX pgbench_accounts_bid_idx ON pgbench_accounts (bid);
--=
CREATE INDEX pgbench_tellers_bid_idx ON pgbench_tellers (bid);

--+ create_procedures
--=
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

--+ analyze
--=
VACUUM ANALYZE pgbench_branches;
--=
VACUUM ANALYZE pgbench_tellers;
--=
VACUUM ANALYZE pgbench_accounts;
--=
VACUUM ANALYZE pgbench_history;

--+ workload
--= tpcb_transaction
SELECT tpcb_transaction(:p_aid, :p_tid, :p_bid, :p_delta)
