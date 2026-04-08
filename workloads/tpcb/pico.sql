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
    bid INTEGER NOT NULL,
    bbalance INTEGER,
    filler VARCHAR(88),
    PRIMARY KEY (bid)
)
--=
CREATE TABLE pgbench_tellers (
    tid INTEGER NOT NULL,
    bid INTEGER,
    tbalance INTEGER,
    filler VARCHAR(84),
    PRIMARY KEY (tid)
)
--=
CREATE TABLE pgbench_accounts (
    aid INTEGER NOT NULL,
    bid INTEGER,
    abalance INTEGER,
    filler VARCHAR(84),
    PRIMARY KEY (aid)
)
--=
CREATE TABLE pgbench_history (
    hid UNSIGNED NOT NULL,
    tid INTEGER,
    bid INTEGER,
    aid INTEGER,
    delta INTEGER,
    mtime DATETIME,
    filler VARCHAR(22),
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
INSERT INTO pgbench_history (hid, tid, bid, aid, delta, mtime, filler) VALUES (:hid, :tid, :bid, :aid, :delta, LOCALTIMESTAMP, '')
