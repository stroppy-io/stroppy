-- workload create_schema
-- query create_accounts_table
CREATE TABLE accounts ( id INTEGER NOT NULL PRIMARY KEY, balance INTEGER );
-- query end
-- query create_history_table
CREATE TABLE history ( account_id INTEGER, amount INTEGER, created_at TIMESTAMP );
-- query end
-- workload end

-- workload insert
-- query insert_accounts
INSERT INTO accounts (id, balance)
VALUES (${accounts.id}, ${accounts.balance});
-- query end
-- workload end

-- workload workload
-- transaction update_and_log
-- query update_balance
UPDATE accounts
SET balance = balance + ${amount}
WHERE id = ${accounts.id};
-- query end

-- query insert_history
INSERT INTO history (account_id, amount, created_at)
VALUES (${accounts.id}, ${amount}, CURRENT_TIMESTAMP);
-- query end
-- transaction end
-- workload end

-- workload cleanup
-- query drop_tables
DROP TABLE IF EXISTS accounts, history CASCADE;
-- query end
-- workload end
