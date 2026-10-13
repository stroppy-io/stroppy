--+ drop_schema
--= drop_bank_account
DROP TABLE IF EXISTS bank_account;

--+ create_schema
--= create_bank_account
CREATE TABLE bank_account (
	id Int64 NOT NULL,
	balance Int64 NOT NULL,
	token Int64 NOT NULL,
	PRIMARY KEY (id)
);
--= create_token_index
CREATE UNIQUE INDEX bank_account_token_uq ON bank_account (token);

--+ workload_tx_transfer
--= read_account
SELECT balance, token FROM bank_account WHERE id = :id
--= read_token
SELECT token FROM bank_account WHERE id = :id
--= rotate_token
UPDATE bank_account SET token = :token WHERE id = :id AND token = :expected
--= move_balance
UPDATE bank_account SET balance = balance + :delta WHERE id = :id

--+ verify
--= total_balance
SELECT coalesce(sum(balance), 0) AS total FROM bank_account
--= token_stats
SELECT count(*) AS accounts_rows,
       coalesce(min(token), 0) AS lowest_token,
       coalesce(max(token), 0) AS highest_token,
       (SELECT count(*) FROM (
            SELECT token FROM bank_account GROUP BY token HAVING count(*) > 1
        ) AS repeated) AS repeated_tokens
FROM bank_account
