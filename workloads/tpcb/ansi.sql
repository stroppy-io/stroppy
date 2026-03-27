--+ cleanup
--= query
DROP TABLE IF EXISTS pgbench_history
--= query
DROP TABLE IF EXISTS pgbench_accounts
--= query
DROP TABLE IF EXISTS pgbench_tellers
--= query
DROP TABLE IF EXISTS pgbench_branches

--+ create_schema
--= query
CREATE TABLE pgbench_branches (
    bid INTEGER NOT NULL PRIMARY KEY,
    bbalance INTEGER,
    filler CHAR(88)
)
--= query
CREATE TABLE pgbench_tellers (
    tid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    tbalance INTEGER,
    filler CHAR(84)
)
--= query
CREATE TABLE pgbench_accounts (
    aid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    abalance INTEGER,
    filler CHAR(84)
)
--= query
CREATE TABLE pgbench_history (
    tid INTEGER,
    bid INTEGER,
    aid INTEGER,
    delta INTEGER,
    mtime TIMESTAMP,
    filler CHAR(22)
)
--= query
CREATE INDEX pgbench_accounts_bid_idx ON pgbench_accounts (bid)
--= query
CREATE INDEX pgbench_tellers_bid_idx ON pgbench_tellers (bid)

--+ workload
--= update_account
UPDATE pgbench_accounts SET abalance = abalance + :delta WHERE aid = :aid
--= get_balance
SELECT abalance FROM pgbench_accounts WHERE aid = :aid
--= update_teller
UPDATE pgbench_tellers SET tbalance = tbalance + :delta WHERE tid = :tid
--= update_branch
UPDATE pgbench_branches SET bbalance = bbalance + :delta WHERE bid = :bid
--= insert_history
INSERT INTO pgbench_history (tid, bid, aid, delta, mtime, filler) VALUES (:tid, :bid, :aid, :delta, CURRENT_TIMESTAMP, '')
