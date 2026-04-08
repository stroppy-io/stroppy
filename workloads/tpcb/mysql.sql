--+ drop_schema
--=
DROP PROCEDURE IF EXISTS tpcb_transaction
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
    bid INT NOT NULL PRIMARY KEY,
    bbalance INT,
    filler CHAR(88)
) ENGINE=InnoDB
--=
CREATE TABLE pgbench_tellers (
    tid INT NOT NULL PRIMARY KEY,
    bid INT,
    tbalance INT,
    filler CHAR(84)
) ENGINE=InnoDB
--=
CREATE TABLE pgbench_accounts (
    aid INT NOT NULL PRIMARY KEY,
    bid INT,
    abalance INT,
    filler CHAR(84)
) ENGINE=InnoDB
--=
CREATE TABLE pgbench_history (
    hid BIGINT NOT NULL PRIMARY KEY,
    tid INT,
    bid INT,
    aid INT,
    delta INT,
    mtime DATETIME,
    filler CHAR(22)
) ENGINE=InnoDB
--=
CREATE INDEX pgbench_accounts_bid_idx ON pgbench_accounts (bid)
--=
CREATE INDEX pgbench_tellers_bid_idx ON pgbench_tellers (bid)


--+ create_procedures
--=
CREATE PROCEDURE tpcb_transaction(
    IN p_aid INT,
    IN p_tid INT,
    IN p_bid INT,
    IN p_delta INT,
    IN p_hid BIGINT
)
BEGIN
    START TRANSACTION;
    UPDATE pgbench_accounts SET abalance = abalance + p_delta WHERE aid = p_aid;
    UPDATE pgbench_tellers  SET tbalance = tbalance + p_delta WHERE tid = p_tid;
    UPDATE pgbench_branches SET bbalance = bbalance + p_delta WHERE bid = p_bid;
    INSERT INTO pgbench_history (hid, tid, bid, aid, delta, mtime, filler)
    VALUES (p_hid, p_tid, p_bid, p_aid, p_delta, NOW(), '');
    COMMIT;
END


--+ workload_procs
--= tpcb_transaction
CALL tpcb_transaction(:p_aid, :p_tid, :p_bid, :p_delta, :p_hid)


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
INSERT INTO pgbench_history (hid, tid, bid, aid, delta, mtime, filler) VALUES (:hid, :tid, :bid, :aid, :delta, NOW(), '')
