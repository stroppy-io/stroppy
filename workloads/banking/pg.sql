--+ drop_schema
--= drop_bank_account
DROP TABLE IF EXISTS bank_account;

--+ create_schema
--= create_bank_account
CREATE TABLE bank_account (
	id bigint NOT NULL PRIMARY KEY,
	balance bigint NOT NULL,
	token bigint NOT NULL UNIQUE
);

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
SELECT coalesce(sum(balance), 0)::bigint AS total FROM bank_account
--= token_stats
SELECT count(*)::bigint AS accounts_rows,
       coalesce(min(token), 0)::bigint AS lowest_token,
       coalesce(max(token), 0)::bigint AS highest_token,
       (SELECT count(*)::bigint FROM (
            SELECT token FROM bank_account GROUP BY token HAVING count(*) > 1
        ) AS repeated) AS repeated_tokens
FROM bank_account

--+ setup_injection
--= create_macavity
CREATE EXTENSION IF NOT EXISTS macavity;

--+ injection
--= arm
SELECT macavity_arm(:point, :action, :occurrence);
