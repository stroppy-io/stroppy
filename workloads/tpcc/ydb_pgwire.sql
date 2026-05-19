-- YDB pgwire (PostgreSQL-compatible wire protocol) dialect for TPC-C.
-- Tested: local-ydb:nightly, YDB_EXPERIMENTAL_PG=1, 2026-04-10.
--
-- === SUPPORTED ===
--   DDL (CREATE/DROP TABLE), single-statement DML, simple-protocol SELECTs,
--   read-only transactions (stock_level, order_status), data querying after
--   loading via native gRPC driver.
--
-- === DEGRADED — workaround available, full spec achievable ===
--   sslmode=disable              — YDB pgwire has no TLS; add to URL
--   simple_protocol required     — extended query protocol portals broken
--                                  ("Portal not found"); fix: append
--                                  default_query_exec_mode=simple_protocol to URL
--   No COPY protocol             — use plain_bulk insert method (~10x slower load)
--   bulkSize must be low (~20)   — wide tables (customer 21 cols x 500 = 10500 params)
--                                  exceed pgwire parameter limit
--   No CASCADE in DROP TABLE     — split into individual DROP TABLE IF EXISTS
--   No REFERENCES (FK)           — omit foreign key constraints from schema
--   No stored procedures         — use tx.ts transaction path only
--   HAS_RETURNING=false required — UPDATE...RETURNING fails ("Portal not found");
--                                  pass -e HAS_RETURNING=false to use separate UPDATE+SELECT
--   No FOR UPDATE                — silently ignored (warning 7000); remove from SQL
--   Auth lockout (4 attempts/1h) — pgx TLS-probe storm triggers brute-force protection;
--                                  fix: mount ydb_auth.txt with attempt_threshold: 0
--                                  via --auth-config-path (see docker-compose.yml)
--
-- === LIMITED — functional with restrictions, not production-grade ===
--   Bulk loading via pgwire is ~10x slower than native gRPC
--   (100k items: ~5 min pgwire vs ~25 s native).
--   Recommended workflow: load data via native driver (-d ydb --no-steps workload),
--   then run workload via pgwire (--steps workload).
--   POOL_SIZE=1 mandatory to avoid auth lockout storms.
--
-- === BLOCKING — TPC-C cannot run full-spec via pgwire ===
--   Multi-statement write transactions break: after BEGIN + SELECT, the next
--   UPDATE/INSERT hits "Transaction not found" (code 2015). This kills new_order
--   and payment txs (~90% error rate in testing). Read-only txs and single-statement
--   delivery work fine. This is a YDB pgwire transaction-handling bug — until fixed,
--   only read-heavy workloads are viable over pgwire.
--
-- Setup:
--   image: ghcr.io/ydb-platform/local-ydb:nightly
--   env:   YDB_EXPERIMENTAL_PG=1, POSTGRES_USER=root, POSTGRES_PASSWORD=1234
--   port:  5432 inside container (map to 5433 on host to avoid clash with pg)
--   auth:  mount ydb_auth.txt { attempt_threshold: 0 } via --auth-config-path
--
-- Example (load via native gRPC, workload via pgwire):
--   K6_SETUP_TIMEOUT=10m ./build/stroppy run tpcc/tx -d ydb --no-steps workload
--   ./build/stroppy run tpcc/tx tpcc/ydb_pgwire -d pg \
--     -D url='postgres://root:1234@localhost:5433/local?sslmode=disable&default_query_exec_mode=simple_protocol' \
--     -D defaultInsertMethod=plain_bulk -D bulkSize=20 \
--     -e POOL_SIZE=1 -e TX_ISOLATION=serializable -e HAS_RETURNING=false \
--     --steps workload

--+ drop_schema
--= drop_order_line
DROP TABLE IF EXISTS order_line
--= drop_new_order
DROP TABLE IF EXISTS new_order
--= drop_orders
DROP TABLE IF EXISTS orders
--= drop_history
DROP TABLE IF EXISTS history
--= drop_stock
DROP TABLE IF EXISTS stock
--= drop_customer
DROP TABLE IF EXISTS customer
--= drop_district
DROP TABLE IF EXISTS district
--= drop_warehouse
DROP TABLE IF EXISTS warehouse
--= drop_item
DROP TABLE IF EXISTS item

--+ create_schema
--= warehouse
CREATE TABLE warehouse (
  w_id INTEGER PRIMARY KEY,
  w_name VARCHAR(10),
  w_street_1 VARCHAR(20),
  w_street_2 VARCHAR(20),
  w_city VARCHAR(20),
  w_state CHAR(2),
  w_zip CHAR(9),
  w_tax DECIMAL(4,4),
  w_ytd DECIMAL(12,2)
)
--= district
CREATE TABLE district (
  d_id INTEGER,
  d_w_id INTEGER,
  d_name VARCHAR(10),
  d_street_1 VARCHAR(20),
  d_street_2 VARCHAR(20),
  d_city VARCHAR(20),
  d_state CHAR(2),
  d_zip CHAR(9),
  d_tax DECIMAL(4,4),
  d_ytd DECIMAL(12,2),
  d_next_o_id INTEGER,
  PRIMARY KEY (d_w_id, d_id)
)
--= customer
CREATE TABLE customer (
  c_id INTEGER,
  c_d_id INTEGER,
  c_w_id INTEGER,
  c_first VARCHAR(16),
  c_middle CHAR(2),
  c_last VARCHAR(16),
  c_street_1 VARCHAR(20),
  c_street_2 VARCHAR(20),
  c_city VARCHAR(20),
  c_state CHAR(2),
  c_zip CHAR(9),
  c_phone CHAR(16),
  c_since TIMESTAMP,
  c_credit CHAR(2),
  c_credit_lim DECIMAL(12,2),
  c_discount DECIMAL(4,4),
  c_balance DECIMAL(12,2),
  c_ytd_payment DECIMAL(12,2),
  c_payment_cnt INTEGER,
  c_delivery_cnt INTEGER,
  c_data VARCHAR(500),
  PRIMARY KEY (c_w_id, c_d_id, c_id)
)
--= history
CREATE TABLE history (
  h_id BIGINT NOT NULL PRIMARY KEY,
  h_c_id INTEGER,
  h_c_d_id INTEGER,
  h_c_w_id INTEGER,
  h_d_id INTEGER,
  h_w_id INTEGER,
  h_date TIMESTAMP,
  h_amount DECIMAL(6,2),
  h_data VARCHAR(24)
)
--= new_order
CREATE TABLE new_order (
  no_o_id INTEGER,
  no_d_id INTEGER,
  no_w_id INTEGER,
  PRIMARY KEY (no_w_id, no_d_id, no_o_id)
)
--= orders
CREATE TABLE orders (
  o_id INTEGER,
  o_d_id INTEGER,
  o_w_id INTEGER,
  o_c_id INTEGER,
  o_entry_d TIMESTAMP,
  o_carrier_id INTEGER,
  o_ol_cnt INTEGER,
  o_all_local INTEGER,
  PRIMARY KEY (o_w_id, o_d_id, o_id)
)
--= order_line
CREATE TABLE order_line (
  ol_o_id INTEGER,
  ol_d_id INTEGER,
  ol_w_id INTEGER,
  ol_number INTEGER,
  ol_i_id INTEGER,
  ol_supply_w_id INTEGER,
  ol_delivery_d TIMESTAMP,
  ol_quantity INTEGER,
  ol_amount DECIMAL(6,2),
  ol_dist_info CHAR(24),
  PRIMARY KEY (ol_w_id, ol_d_id, ol_o_id, ol_number)
)
--= item
CREATE TABLE item (
  i_id INTEGER PRIMARY KEY,
  i_im_id INTEGER,
  i_name VARCHAR(24),
  i_price DECIMAL(5,2),
  i_data VARCHAR(50)
)
--= stock
CREATE TABLE stock (
  s_i_id INTEGER,
  s_w_id INTEGER,
  s_quantity INTEGER,
  s_dist_01 CHAR(24),
  s_dist_02 CHAR(24),
  s_dist_03 CHAR(24),
  s_dist_04 CHAR(24),
  s_dist_05 CHAR(24),
  s_dist_06 CHAR(24),
  s_dist_07 CHAR(24),
  s_dist_08 CHAR(24),
  s_dist_09 CHAR(24),
  s_dist_10 CHAR(24),
  s_ytd INTEGER,
  s_order_cnt INTEGER,
  s_remote_cnt INTEGER,
  s_data VARCHAR(50),
  PRIMARY KEY (s_w_id, s_i_id)
)

--+ workload_tx_new_order
--= get_customer
SELECT c_discount, c_last, c_credit FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= get_warehouse
SELECT w_tax FROM warehouse WHERE w_id = :w_id
--= get_district
SELECT d_next_o_id, d_tax FROM district WHERE d_id = :d_id AND d_w_id = :w_id
--= update_district
UPDATE district SET d_next_o_id = d_next_o_id + 1 WHERE d_id = :d_id AND d_w_id = :w_id
--= insert_order
INSERT INTO orders (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)
VALUES (:o_id, :d_id, :w_id, :c_id, current_timestamp, :ol_cnt, :all_local)
--= insert_new_order
INSERT INTO new_order (no_o_id, no_d_id, no_w_id) VALUES (:o_id, :d_id, :w_id)
--= get_item
SELECT i_price, i_name, i_data FROM item WHERE i_id = :i_id
--= get_stock
SELECT s_quantity, s_data, s_dist_01, s_dist_02, s_dist_03, s_dist_04, s_dist_05, s_dist_06, s_dist_07, s_dist_08, s_dist_09, s_dist_10
FROM stock WHERE s_i_id = :i_id AND s_w_id = :w_id
--= update_stock
UPDATE stock SET s_quantity = :quantity, s_ytd = s_ytd + :ol_quantity, s_order_cnt = s_order_cnt + 1, s_remote_cnt = s_remote_cnt + :remote_cnt
WHERE s_i_id = :i_id AND s_w_id = :w_id
--= insert_order_line
INSERT INTO order_line (ol_o_id, ol_d_id, ol_w_id, ol_number, ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_dist_info)
VALUES (:o_id, :d_id, :w_id, :ol_number, :i_id, :supply_w_id, :quantity, :amount, :dist_info)
--= get_items_batch
SELECT i_id, i_price, i_name, i_data FROM item WHERE i_id IN ({item_ids})
--= get_stocks_batch
SELECT s_i_id, s_quantity, s_data, s_dist_01, s_dist_02, s_dist_03, s_dist_04, s_dist_05, s_dist_06, s_dist_07, s_dist_08, s_dist_09, s_dist_10
FROM stock WHERE s_w_id = :w_id AND s_i_id IN ({item_ids})

--+ workload_tx_payment
--= update_warehouse
UPDATE warehouse SET w_ytd = w_ytd + :amount WHERE w_id = :w_id
--= get_warehouse
SELECT w_name, w_street_1, w_street_2, w_city, w_state, w_zip FROM warehouse WHERE w_id = :w_id
--= update_get_warehouse
UPDATE warehouse SET w_ytd = w_ytd + :amount WHERE w_id = :w_id
RETURNING w_name, w_street_1, w_street_2, w_city, w_state, w_zip
--= update_district
UPDATE district SET d_ytd = d_ytd + :amount WHERE d_w_id = :w_id AND d_id = :d_id
--= get_district
SELECT d_name, d_street_1, d_street_2, d_city, d_state, d_zip FROM district WHERE d_w_id = :w_id AND d_id = :d_id
--= update_get_district
UPDATE district SET d_ytd = d_ytd + :amount WHERE d_w_id = :w_id AND d_id = :d_id
RETURNING d_name, d_street_1, d_street_2, d_city, d_state, d_zip
--= get_customer_by_id
SELECT c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since, c_data
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= count_customers_by_name
SELECT COUNT(*) FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
--= get_customer_by_name
SELECT c_id, c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since, c_data
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
ORDER BY c_first
LIMIT 1 OFFSET :offset
--= update_customer
UPDATE customer SET c_balance = c_balance - :amount, c_ytd_payment = c_ytd_payment + :amount, c_payment_cnt = c_payment_cnt + 1
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= update_customer_bc
UPDATE customer
   SET c_balance     = c_balance - :amount,
       c_ytd_payment = c_ytd_payment + :amount,
       c_payment_cnt = c_payment_cnt + 1,
       c_data        = SUBSTR(:c_data_new, 1, 500)
 WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= insert_history
INSERT INTO history (h_id, h_c_id, h_c_d_id, h_c_w_id, h_d_id, h_w_id, h_date, h_amount, h_data)
VALUES (:h_id, :h_c_id, :h_c_d_id, :h_c_w_id, :h_d_id, :h_w_id, current_timestamp, :h_amount, :h_data)

--+ workload_tx_order_status
--= get_customer_by_id
SELECT c_balance, c_first, c_middle, c_last, c_id FROM customer WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id
--= count_customers_by_name
SELECT COUNT(*) FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
--= get_customer_by_name
SELECT c_balance, c_first, c_middle, c_last, c_id FROM customer
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
ORDER BY c_first
LIMIT 1 OFFSET :offset
--= get_last_order
SELECT o_id, o_carrier_id, o_entry_d FROM orders WHERE o_d_id = :d_id AND o_w_id = :w_id AND o_c_id = :c_id ORDER BY o_id DESC LIMIT 1
--= get_order_lines
SELECT ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_delivery_d FROM order_line WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id

--+ workload_tx_delivery
--= get_min_new_order
SELECT min(no_o_id) FROM new_order WHERE no_d_id = :d_id AND no_w_id = :w_id
--= delete_new_order
DELETE FROM new_order WHERE no_o_id = :o_id AND no_d_id = :d_id AND no_w_id = :w_id
--= get_order
SELECT o_c_id FROM orders WHERE o_id = :o_id AND o_d_id = :d_id AND o_w_id = :w_id
--= update_order
UPDATE orders SET o_carrier_id = :carrier_id WHERE o_id = :o_id AND o_d_id = :d_id AND o_w_id = :w_id
--= update_order_line
UPDATE order_line SET ol_delivery_d = current_timestamp WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= get_order_line_amount
SELECT SUM(ol_amount) FROM order_line WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= update_customer
UPDATE customer SET c_balance = c_balance + :amount, c_delivery_cnt = c_delivery_cnt + 1 WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id

--+ workload_tx_stock_level
--= get_district
SELECT d_next_o_id FROM district WHERE d_w_id = :w_id AND d_id = :d_id
--= get_window_items
SELECT DISTINCT ol_i_id FROM order_line
WHERE ol_w_id = :w_id
  AND ol_d_id = :d_id
  AND ol_o_id >= :min_o_id
  AND ol_o_id < :next_o_id
--= stock_count_in
SELECT COUNT(*) FROM stock
WHERE s_w_id = :w_id
  AND s_quantity < :threshold
  AND s_i_id IN ({ids})
