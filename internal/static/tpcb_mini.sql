-- workload create_schema
-- query create_accounts_table
CREATE TABLE accounts (
    id INTEGER NOT NULL PRIMARY KEY,
    balance INTEGER
);
-- query end

-- query create_history_table
CREATE TABLE history (
    account_id INTEGER,
    amount INTEGER,
    created_at TIMESTAMP
);
-- query end
-- workload end


-- workload insert
-- query insert_accounts
INSERT INTO accounts (id, balance)
VALUES (1, 1000), (2, 2000);
-- query end
-- workload end


-- workload workload
-- transaction update_and_log
-- query update_balance
UPDATE accounts
SET balance = balance + 100
WHERE id = 1;
-- query end

-- query select_balance
SELECT balance
FROM accounts
WHERE id = 1;
-- query end

-- query insert_history
INSERT INTO history (account_id, amount, created_at)
VALUES (1, 100, CURRENT_TIMESTAMP);
-- query end
-- transaction end
-- workload end

-- workload cleanup
-- query drop_history
DROP TABLE IF EXISTS history CASCADE;
-- query end

-- query drop_accounts
DROP TABLE IF EXISTS accounts CASCADE;
-- query end
-- workload end
