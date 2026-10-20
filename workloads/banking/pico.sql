-- Picodata cannot carry this workload's second witness, and the reasons are the
-- engine's own rules — not a gap in this file:
--
--   * Unique indexes are only allowed on sharding-key columns (UNIQUE is
--     "доступно только для индекса TREE по колонкам ключа шардирования"): a
--     uniqueness constraint on `token` is not expressible, so it cannot witness a
--     lost or duplicated transfer.
--   * Transactions are `DO $$ … END $$` blocks, and every statement inside a block
--     must touch one bucket (one sharding key). A transfer moves money between two
--     accounts, which are two sharding keys, so the two-row swap cannot be atomic —
--     there is no way to make it all-or-nothing.
--   * There is no HAVING in the SQL command index, so the repeat check used on the
--     other dialects has no equivalent here.
--
-- So this asset does the transfer as two relative updates and the verify step
-- checks the total balance only. The token column is still loaded — the schema
-- keeps its shape and the load path is shared — but nothing depends on it, and the
-- run says so in its report. The guarantee here is weaker: money is still
-- conserved by each statement, but a transfer interrupted between the two updates
-- is not rolled back, and a repeated swap is not detectable.

--+ drop_schema
--= drop_bank_account
DROP TABLE IF EXISTS bank_account;

--+ create_schema
--= create_bank_account
CREATE TABLE bank_account (
	id BIGINT NOT NULL,
	balance BIGINT NOT NULL,
	token BIGINT NOT NULL,
	PRIMARY KEY (id)
);

--+ workload_tx_transfer
--= move_balance
UPDATE bank_account SET balance = balance + :delta WHERE id = :id

--+ verify
--= total_balance
SELECT COALESCE(SUM(balance), 0) AS total FROM bank_account
