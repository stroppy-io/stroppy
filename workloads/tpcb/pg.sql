--+ drop_schema
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
    hid BIGINT NOT NULL PRIMARY KEY,
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
    p_delta INTEGER,
    p_hid BIGINT
)
RETURNS INTEGER
LANGUAGE plpgsql
AS $$
DECLARE
    v_balance INTEGER;
BEGIN
    UPDATE pgbench_accounts
    SET abalance = abalance + p_delta
    WHERE pgbench_accounts.aid = p_aid;

    SELECT pgbench_accounts.abalance INTO v_balance
    FROM pgbench_accounts
    WHERE pgbench_accounts.aid = p_aid;

    UPDATE pgbench_tellers
    SET tbalance = tbalance + p_delta
    WHERE pgbench_tellers.tid = p_tid;

    UPDATE pgbench_branches
    SET bbalance = bbalance + p_delta
    WHERE pgbench_branches.bid = p_bid;

    INSERT INTO pgbench_history (hid, tid, bid, aid, delta, mtime, filler)
    VALUES (p_hid, p_tid, p_bid, p_aid, p_delta, CURRENT_TIMESTAMP, '');

    RETURN v_balance;
END;
$$;


--+ workload_procs
--= tpcb_transaction
SELECT tpcb_transaction(:p_aid, :p_tid, :p_bid, :p_delta, :p_hid)


--+ workload_tx_tpcb
--= update_account
UPDATE pgbench_accounts SET abalance = abalance + :delta WHERE aid = :aid
--= get_balance
SELECT abalance FROM pgbench_accounts WHERE aid = :aid
--= update_teller
UPDATE pgbench_tellers SET tbalance = tbalance + :delta WHERE tid = :tid
--= update_branch
UPDATE pgbench_branches SET bbalance = bbalance + :delta WHERE bid = :bid
--= insert_history
INSERT INTO pgbench_history (hid, tid, bid, aid, delta, mtime, filler) VALUES (:hid, :tid, :bid, :aid, :delta, CURRENT_TIMESTAMP, '')
