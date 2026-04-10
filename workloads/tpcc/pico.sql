--+ drop_schema
--=
DROP TABLE IF EXISTS order_line
--=
DROP TABLE IF EXISTS new_order
--=
DROP TABLE IF EXISTS orders
--=
DROP TABLE IF EXISTS history
--=
DROP TABLE IF EXISTS stock
--=
DROP TABLE IF EXISTS customer
--=
DROP TABLE IF EXISTS district
--=
DROP TABLE IF EXISTS warehouse
--=
DROP TABLE IF EXISTS item

--+ create_schema
--= warehouse
CREATE TABLE warehouse (
    w_id INTEGER NOT NULL,
    w_name VARCHAR(10),
    w_street_1 VARCHAR(20),
    w_street_2 VARCHAR(20),
    w_city VARCHAR(20),
    w_state VARCHAR(2),
    w_zip VARCHAR(9),
    w_tax DECIMAL(4,4),
    w_ytd DECIMAL(12,2),
    PRIMARY KEY (w_id)
)
--= district
CREATE TABLE district (
    d_w_id INTEGER NOT NULL,
    d_id INTEGER NOT NULL,
    d_name VARCHAR(10),
    d_street_1 VARCHAR(20),
    d_street_2 VARCHAR(20),
    d_city VARCHAR(20),
    d_state VARCHAR(2),
    d_zip VARCHAR(9),
    d_tax DECIMAL(4,4),
    d_ytd DECIMAL(12,2),
    d_next_o_id INTEGER,
    PRIMARY KEY (d_w_id, d_id)
)
--= customer
CREATE TABLE customer (
    c_w_id INTEGER NOT NULL,
    c_d_id INTEGER NOT NULL,
    c_id INTEGER NOT NULL,
    c_first VARCHAR(16),
    c_middle VARCHAR(2),
    c_last VARCHAR(16),
    c_street_1 VARCHAR(20),
    c_street_2 VARCHAR(20),
    c_city VARCHAR(20),
    c_state VARCHAR(2),
    c_zip VARCHAR(9),
    c_phone VARCHAR(16),
    c_since DATETIME,
    c_credit VARCHAR(2),
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
    h_id BIGINT NOT NULL,
    h_c_id INTEGER,
    h_c_d_id INTEGER,
    h_c_w_id INTEGER,
    h_d_id INTEGER,
    h_w_id INTEGER,
    h_date DATETIME,
    h_amount DECIMAL(6,2),
    h_data VARCHAR(24),
    PRIMARY KEY (h_id)
)
--= new_order
CREATE TABLE new_order (
    no_w_id INTEGER NOT NULL,
    no_d_id INTEGER NOT NULL,
    no_o_id INTEGER NOT NULL,
    PRIMARY KEY (no_w_id, no_d_id, no_o_id)
)
--= orders
CREATE TABLE orders (
    o_w_id INTEGER NOT NULL,
    o_d_id INTEGER NOT NULL,
    o_id INTEGER NOT NULL,
    o_c_id INTEGER,
    o_entry_d DATETIME,
    o_carrier_id INTEGER,
    o_ol_cnt INTEGER,
    o_all_local INTEGER,
    PRIMARY KEY (o_w_id, o_d_id, o_id)
)
--= order_line
CREATE TABLE order_line (
    ol_w_id INTEGER NOT NULL,
    ol_d_id INTEGER NOT NULL,
    ol_o_id INTEGER NOT NULL,
    ol_number INTEGER NOT NULL,
    ol_i_id INTEGER,
    ol_supply_w_id INTEGER,
    ol_delivery_d DATETIME,
    ol_quantity INTEGER,
    ol_amount DECIMAL(6,2),
    ol_dist_info VARCHAR(24),
    PRIMARY KEY (ol_w_id, ol_d_id, ol_o_id, ol_number)
)
--= item
CREATE TABLE item (
    i_id INTEGER NOT NULL,
    i_im_id INTEGER,
    i_name VARCHAR(24),
    i_price DECIMAL(5,2),
    i_data VARCHAR(50),
    PRIMARY KEY (i_id)
)
--= stock
CREATE TABLE stock (
    s_w_id INTEGER NOT NULL,
    s_i_id INTEGER NOT NULL,
    s_quantity INTEGER,
    s_dist_01 VARCHAR(24),
    s_dist_02 VARCHAR(24),
    s_dist_03 VARCHAR(24),
    s_dist_04 VARCHAR(24),
    s_dist_05 VARCHAR(24),
    s_dist_06 VARCHAR(24),
    s_dist_07 VARCHAR(24),
    s_dist_08 VARCHAR(24),
    s_dist_09 VARCHAR(24),
    s_dist_10 VARCHAR(24),
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
VALUES (:o_id, :d_id, :w_id, :c_id, LOCALTIMESTAMP, :ol_cnt, :all_local)
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
--= update_district
UPDATE district SET d_ytd = d_ytd + :amount WHERE d_w_id = :w_id AND d_id = :d_id
--= get_district
SELECT d_name, d_street_1, d_street_2, d_city, d_state, d_zip FROM district WHERE d_w_id = :w_id AND d_id = :d_id
--= get_customer_by_id
-- Trailing c_data is needed for the §2.5.2.2 BC-credit append path.
SELECT c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since, c_data
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= count_customers_by_name
-- TPC-C 2.5.1.2: 60% of Payment lookups are by (w_id, d_id, c_last).
-- Single-table SELECT — safe for sbroad (no cross-shard motion).
-- Using -- comments (not /* */) because sbroad's parser rejects block
-- comments at the head of a statement; parse_sql strips -- lines from
-- query bodies before the query reaches picodata.
SELECT COUNT(*) FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
--= get_customer_by_name
-- TPC-C 2.5.2.2: pick row ceil(n/2) ordered by c_first.
-- picodata/sbroad rejects OFFSET in SELECT ("expected EOI or DqlOption"),
-- so we fetch all matching rows and pick the median in tx.ts (IS_PICODATA
-- branch). Trailing c_data supports the BC-credit append path (§1.8).
SELECT c_id, c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since, c_data
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
ORDER BY c_first
--= update_customer
UPDATE customer SET c_balance = c_balance - :amount, c_ytd_payment = c_ytd_payment + :amount, c_payment_cnt = c_payment_cnt + 1
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= update_customer_bc
-- TPC-C 2.5.2.2: BC-credit path. c_data_new is built client-side
-- (c_id c_d_id c_w_id d_id w_id h_amount|old_c_data); SUBSTR clamps
-- to 500 chars. Picodata/sbroad accepts SUBSTR(expr, 1, 500).
UPDATE customer
   SET c_balance     = c_balance - :amount,
       c_ytd_payment = c_ytd_payment + :amount,
       c_payment_cnt = c_payment_cnt + 1,
       c_data        = SUBSTR(:c_data_new, 1, 500)
 WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= insert_history
INSERT INTO history (h_id, h_c_id, h_c_d_id, h_c_w_id, h_d_id, h_w_id, h_date, h_amount, h_data)
VALUES (:h_id, :h_c_id, :h_c_d_id, :h_c_w_id, :h_d_id, :h_w_id, LOCALTIMESTAMP, :h_amount, :h_data)

--+ workload_tx_order_status
--= get_customer_by_id
SELECT c_balance, c_first, c_middle, c_last, c_id FROM customer WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id
--= count_customers_by_name
-- TPC-C 2.6.1.2: 60% of Order-Status lookups are by (w_id, d_id, c_last).
SELECT COUNT(*) FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
--= get_customer_by_name
-- TPC-C 2.6.2.2: pick row ceil(n/2) ordered by c_first.
-- picodata/sbroad rejects OFFSET; fetch all rows and pick in tx.ts.
SELECT c_balance, c_first, c_middle, c_last, c_id FROM customer
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
ORDER BY c_first
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
UPDATE order_line SET ol_delivery_d = LOCALTIMESTAMP WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= get_order_line_amount
SELECT SUM(ol_amount) FROM order_line WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= update_customer
UPDATE customer SET c_balance = c_balance + :amount, c_delivery_cnt = c_delivery_cnt + 1 WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id

--+ workload_tx_stock_level
--= get_district
SELECT d_next_o_id FROM district WHERE d_w_id = :w_id AND d_id = :d_id
--= get_window_items
-- Step 1 of the picodata stock_level rewrite: read the distinct order_line
-- item ids from the last-20-orders window. Picodata's sbroad planner
-- intermittently fails the single-query JOIN/subquery form with "Temporary
-- SQL table TMP_... not found" (unused-motion cleanup race), so we split
-- the scan into two steps and count low-stock matches in the script.
SELECT DISTINCT ol_i_id FROM order_line
WHERE ol_w_id = :w_id
  AND ol_d_id = :d_id
  AND ol_o_id >= :min_o_id
  AND ol_o_id < :next_o_id
--= stock_count_in
-- Step 2: count low-stock items from an inline IN(...) list. Stroppy's
-- :name substitution leaves the IN list alone, so we interpolate the ids
-- directly — they come from the previous trusted SELECT, not user input.
-- {ids} is replaced in the TypeScript before the query is handed to exec.
SELECT COUNT(*) FROM stock
WHERE s_w_id = :w_id
  AND s_quantity < :threshold
  AND s_i_id IN ({ids})
