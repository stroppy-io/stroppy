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


--+ create_indexes
-- Built AFTER the bulk load: a one-shot index build is far cheaper than
-- maintaining the index per row during population.
--= accounts_bid
CREATE INDEX pgbench_accounts_bid_idx ON pgbench_accounts (bid)
--= tellers_bid
CREATE INDEX pgbench_tellers_bid_idx ON pgbench_tellers (bid)


--+ create_foreign_keys
-- pgbench --foreign-keys schema: bid/tid/aid reference their parent tables.
-- Added post-load; InnoDB requires (and the accounts/tellers bid indexes above
-- provide, others it auto-creates) a child-side index for each FK.
--= tellers_bid_fk
ALTER TABLE pgbench_tellers  ADD CONSTRAINT pgbench_tellers_bid_fk  FOREIGN KEY (bid) REFERENCES pgbench_branches (bid)
--= accounts_bid_fk
ALTER TABLE pgbench_accounts ADD CONSTRAINT pgbench_accounts_bid_fk FOREIGN KEY (bid) REFERENCES pgbench_branches (bid)
--= history_bid_fk
ALTER TABLE pgbench_history  ADD CONSTRAINT pgbench_history_bid_fk  FOREIGN KEY (bid) REFERENCES pgbench_branches (bid)
--= history_tid_fk
ALTER TABLE pgbench_history  ADD CONSTRAINT pgbench_history_tid_fk  FOREIGN KEY (tid) REFERENCES pgbench_tellers (tid)
--= history_aid_fk
ALTER TABLE pgbench_history  ADD CONSTRAINT pgbench_history_aid_fk  FOREIGN KEY (aid) REFERENCES pgbench_accounts (aid)


--+ analyze
-- Refresh planner statistics after the bulk load.
--=
ANALYZE TABLE pgbench_branches, pgbench_tellers, pgbench_accounts, pgbench_history


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
