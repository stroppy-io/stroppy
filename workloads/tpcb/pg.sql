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


--+ set_unlogged
-- Flip tables to UNLOGGED for a WAL-free bulk load; set_logged restores
-- durability after population. Gated by PG_UNLOGGED (default true), pg-only.
--= branches
ALTER TABLE pgbench_branches SET UNLOGGED;
--= tellers
ALTER TABLE pgbench_tellers  SET UNLOGGED;
--= accounts
ALTER TABLE pgbench_accounts SET UNLOGGED;
--= history
ALTER TABLE pgbench_history  SET UNLOGGED;


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


--+ create_indexes
-- Built AFTER the bulk load: a one-shot index build is far cheaper than
-- maintaining the index per row during population.
--= accounts_bid
CREATE INDEX pgbench_accounts_bid_idx ON pgbench_accounts (bid);
--= tellers_bid
CREATE INDEX pgbench_tellers_bid_idx ON pgbench_tellers (bid);


--+ create_foreign_keys
-- pgbench --foreign-keys schema: the bid/tid/aid columns reference their parent
-- tables. Added post-load (referenced rows already present); the bid indexes
-- above back the accounts/tellers -> branches references.
--= tellers_bid_fk
ALTER TABLE pgbench_tellers  ADD CONSTRAINT pgbench_tellers_bid_fkey  FOREIGN KEY (bid) REFERENCES pgbench_branches (bid);
--= accounts_bid_fk
ALTER TABLE pgbench_accounts ADD CONSTRAINT pgbench_accounts_bid_fkey FOREIGN KEY (bid) REFERENCES pgbench_branches (bid);
--= history_bid_fk
ALTER TABLE pgbench_history  ADD CONSTRAINT pgbench_history_bid_fkey  FOREIGN KEY (bid) REFERENCES pgbench_branches (bid);
--= history_tid_fk
ALTER TABLE pgbench_history  ADD CONSTRAINT pgbench_history_tid_fkey  FOREIGN KEY (tid) REFERENCES pgbench_tellers (tid);
--= history_aid_fk
ALTER TABLE pgbench_history  ADD CONSTRAINT pgbench_history_aid_fkey  FOREIGN KEY (aid) REFERENCES pgbench_accounts (aid);


--+ set_logged
-- Restore durability after the UNLOGGED bulk load (pg-only, PG_UNLOGGED).
--= branches
ALTER TABLE pgbench_branches SET LOGGED;
--= tellers
ALTER TABLE pgbench_tellers  SET LOGGED;
--= accounts
ALTER TABLE pgbench_accounts SET LOGGED;
--= history
ALTER TABLE pgbench_history  SET LOGGED;


--+ analyze
-- Refresh planner statistics after the bulk load.
--=
ANALYZE;


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
