--+ drop_schema
--=
DROP TABLE IF EXISTS pgbench_history
--=
DROP TABLE IF EXISTS pgbench_accounts
--=
DROP TABLE IF EXISTS pgbench_tellers
--=
DROP TABLE IF EXISTS pgbench_branches


--+ create_schema
--=
CREATE TABLE pgbench_branches (
    bid Int64 NOT NULL,
    bbalance Int64,
    filler Utf8,
    PRIMARY KEY (bid)
)
--=
CREATE TABLE pgbench_tellers (
    tid Int64 NOT NULL,
    bid Int64,
    tbalance Int64,
    filler Utf8,
    PRIMARY KEY (tid)
)
--=
CREATE TABLE pgbench_accounts (
    aid Int64 NOT NULL,
    bid Int64,
    abalance Int64,
    filler Utf8,
    PRIMARY KEY (aid)
)
--=
CREATE TABLE pgbench_history (
    hid Int64 NOT NULL,
    tid Int64,
    bid Int64,
    aid Int64,
    delta Int64,
    mtime Timestamp,
    filler Utf8,
    PRIMARY KEY (hid)
)


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
UPSERT INTO pgbench_history (hid, tid, bid, aid, delta, mtime, filler) VALUES (:hid, :tid, :bid, :aid, :delta, CurrentUtcTimestamp(), '')
