-- section create_schema

-- query
CREATE TABLE accounts (
    id INTEGER NOT NULL PRIMARY KEY,
    balance INTEGER
);
-- query end

-- query
CREATE TABLE history (
    account_id INTEGER,
    amount INTEGER,
    created_at TIMESTAMP
);
-- query end
-- section end

-- section insert

-- query
INSERT INTO accounts (id, balance)
VALUES (1, 1000), (2, 2000);
-- query end
-- section end

-- section workload
-- transaction
-- query end

-- query
UPDATE accounts
SET balance = balance + 100
WHERE id = 1;
-- query end

-- query
SELECT balance
FROM accounts
WHERE id = 1;
-- query end

-- query
INSERT INTO history (account_id, amount, created_at)
VALUES (1, 100, CURRENT_TIMESTAMP);
-- query end
-- transaction end
-- section end

-- section cleanup
-- query
DROP TABLE IF EXISTS history CASCADE;
-- query end

-- query
DROP TABLE IF EXISTS accounts CASCADE;
-- query end
-- section end
