-- TiDB speaks the mysql wire but has no stored procedures, so this variant omits the
-- three pieces the mysql asset carries for them: the five DROP PROCEDURE statements,
-- the create_procedures section, and the workload_procs section. Everything else is
-- identical, including SET FOREIGN_KEY_CHECKS = 0, which TiDB accepts.
--
-- Measured: without this, tpcc/tx on TiDB fails in drop_schema with
-- `Error 8108 (HY000): Unsupported type *resolve.NodeW`, the same first-statement
-- failure TPC-B had. TPC-C's procedure variant is therefore not available on TiDB.
--
-- Running it against a single-node TiDB needs one further step, also measured. Seed a
-- cheap samplerate before measuring, or `ANALYZE TABLE` over nine tables takes minutes
-- each and exhausts the box — with it, all nine finished in about a second, against
-- 6m54s without:
--
--   SET GLOBAL tidb_server_memory_limit='1000MB';
--   ANALYZE TABLE warehouse, district, customer, history, new_order, orders,
--                 order_line, item, stock WITH 0.02 SAMPLERATE;
--   stroppy run tpcc/tx -d tidb --steps analyze,workload
--
-- Measured that way the run exits 0: 704 transactions, 701 commits, all five transaction
-- types non-zero, and 295 rows in `history`, which only payments write. Start the
-- playground with `--tiflash 0` and pre-download its components: that keeps it to three
-- servers (tikv, tidb, pd), which is what fits.

--+ drop_schema
--= set_foreign_keys
SET FOREIGN_KEY_CHECKS = 0
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
  w_id INT NOT NULL PRIMARY KEY,
  w_name VARCHAR(10),
  w_street_1 VARCHAR(20),
  w_street_2 VARCHAR(20),
  w_city VARCHAR(20),
  w_state CHAR(2),
  w_zip CHAR(9),
  w_tax DECIMAL(4,4),
  w_ytd DECIMAL(12,2)
) ENGINE=InnoDB
--= district
CREATE TABLE district (
  d_id INT NOT NULL,
  d_w_id INT NOT NULL,
  d_name VARCHAR(10),
  d_street_1 VARCHAR(20),
  d_street_2 VARCHAR(20),
  d_city VARCHAR(20),
  d_state CHAR(2),
  d_zip CHAR(9),
  d_tax DECIMAL(4,4),
  d_ytd DECIMAL(12,2),
  d_next_o_id INT,
  PRIMARY KEY (d_w_id, d_id)
) ENGINE=InnoDB
--= customer
CREATE TABLE customer (
  c_id INT NOT NULL,
  c_d_id INT NOT NULL,
  c_w_id INT NOT NULL,
  c_first VARCHAR(16),
  c_middle CHAR(2),
  c_last VARCHAR(16),
  c_street_1 VARCHAR(20),
  c_street_2 VARCHAR(20),
  c_city VARCHAR(20),
  c_state CHAR(2),
  c_zip CHAR(9),
  c_phone CHAR(16),
  c_since DATETIME,
  c_credit CHAR(2),
  c_credit_lim DECIMAL(12,2),
  c_discount DECIMAL(4,4),
  c_balance DECIMAL(12,2),
  c_ytd_payment DECIMAL(12,2),
  c_payment_cnt INT,
  c_delivery_cnt INT,
  c_data VARCHAR(500),
  PRIMARY KEY (c_w_id, c_d_id, c_id)
) ENGINE=InnoDB
--= history
CREATE TABLE history (
  h_id BIGINT NOT NULL PRIMARY KEY,
  h_c_id INT,
  h_c_d_id INT,
  h_c_w_id INT,
  h_d_id INT,
  h_w_id INT,
  h_date DATETIME,
  h_amount DECIMAL(6,2),
  h_data VARCHAR(24)
) ENGINE=InnoDB
--= new_order
CREATE TABLE new_order (
  no_o_id INT NOT NULL,
  no_d_id INT NOT NULL,
  no_w_id INT NOT NULL,
  PRIMARY KEY (no_w_id, no_d_id, no_o_id)
) ENGINE=InnoDB
--= orders
CREATE TABLE orders (
  o_id INT NOT NULL,
  o_d_id INT NOT NULL,
  o_w_id INT NOT NULL,
  o_c_id INT,
  o_entry_d DATETIME,
  o_carrier_id INT,
  o_ol_cnt INT,
  o_all_local INT,
  PRIMARY KEY (o_w_id, o_d_id, o_id)
) ENGINE=InnoDB
--= order_line
CREATE TABLE order_line (
  ol_o_id INT NOT NULL,
  ol_d_id INT NOT NULL,
  ol_w_id INT NOT NULL,
  ol_number INT NOT NULL,
  ol_i_id INT,
  ol_supply_w_id INT,
  ol_delivery_d DATETIME,
  ol_quantity INT,
  ol_amount DECIMAL(6,2),
  ol_dist_info CHAR(24),
  PRIMARY KEY (ol_w_id, ol_d_id, ol_o_id, ol_number)
) ENGINE=InnoDB
--= item
CREATE TABLE item (
  i_id INT NOT NULL PRIMARY KEY,
  i_im_id INT,
  i_name VARCHAR(24),
  i_price DECIMAL(5,2),
  i_data VARCHAR(50)
) ENGINE=InnoDB
--= stock
CREATE TABLE stock (
  s_i_id INT NOT NULL,
  s_w_id INT NOT NULL,
  s_quantity INT,
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
  s_ytd INT,
  s_order_cnt INT,
  s_remote_cnt INT,
  s_data VARCHAR(50),
  PRIMARY KEY (s_w_id, s_i_id)
) ENGINE=InnoDB

--+ create_indexes
-- TPC-C secondary indexes (spec-permitted, Clause 1.4): the C_LAST by-name path
-- (Payment/Order-Status) and the customer's-latest-order lookup. Built post-load
-- to keep write amplification out of the bulk-load path; mirror the ydb dialect.
--= idx_customer_name
CREATE INDEX idx_customer_name ON customer (c_w_id, c_d_id, c_last, c_first)
--= idx_order
CREATE INDEX idx_order ON orders (o_w_id, o_d_id, o_c_id, o_id)


--+ analyze
-- Refresh planner statistics after the bulk load.
--=
ANALYZE TABLE warehouse, district, customer, history, new_order, orders, order_line, item, stock


--+ workload_tx_new_order
--= get_customer
SELECT c_discount, c_last, c_credit FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= get_warehouse
SELECT w_tax FROM warehouse WHERE w_id = :w_id
--= get_district
/* T2.1: FOR UPDATE serializes the read-then-increment of d_next_o_id under
   InnoDB. Without it, REPEATABLE READ does a consistent (snapshot) read so
   two concurrent NEWORD VUs can read the same d_next_o_id, both compute the
   same o_id, and both INSERT INTO orders fail with Duplicate entry on the
   PK. The lock is released on commit/rollback. Mirrors the proc-body fix
   in NEWORD (workload_procs section above) for the inline-SQL variant. */
SELECT d_next_o_id, d_tax FROM district WHERE d_id = :d_id AND d_w_id = :w_id FOR UPDATE
--= update_district
UPDATE district SET d_next_o_id = d_next_o_id + 1 WHERE d_id = :d_id AND d_w_id = :w_id
--= insert_order
INSERT INTO orders (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)
VALUES (:o_id, :d_id, :w_id, :c_id, NOW(), :ol_cnt, :all_local)
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
/* Trailing c_data is needed for the §2.5.2.2 BC-credit append path. */
SELECT c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since, c_data
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= count_customers_by_name
/* TPC-C 2.5.1.2: 60% of Payment lookups are by (w_id, d_id, c_last). */
SELECT COUNT(*) FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
--= get_customer_by_name
/* TPC-C 2.5.2.2: pick row ceil(n/2) ordered by c_first — zero-indexed
   OFFSET is (n - 1) / 2, computed client-side and passed in.
   MySQL accepts LIMIT/OFFSET with protocol parameters inside a
   prepared statement; a named-colon token works here (it errors
   only on literal non-integer expressions).
   Trailing c_data supports the BC-credit append path (§1.8). */
SELECT c_id, c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since, c_data
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
ORDER BY c_first
LIMIT 1 OFFSET :offset
--= update_customer
UPDATE customer SET c_balance = c_balance - :amount, c_ytd_payment = c_ytd_payment + :amount, c_payment_cnt = c_payment_cnt + 1
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= update_customer_bc
/* TPC-C 2.5.2.2: BC-credit path. c_data_new is built client-side
   (c_id c_d_id c_w_id d_id w_id h_amount|old_c_data); SUBSTR clamps
   to 500 chars. MySQL SUBSTR is 1-indexed, same as pg/pico/ydb. */
UPDATE customer
   SET c_balance     = c_balance - :amount,
       c_ytd_payment = c_ytd_payment + :amount,
       c_payment_cnt = c_payment_cnt + 1,
       c_data        = SUBSTR(:c_data_new, 1, 500)
 WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= insert_history
INSERT INTO history (h_id, h_c_id, h_c_d_id, h_c_w_id, h_d_id, h_w_id, h_date, h_amount, h_data)
VALUES (:h_id, :h_c_id, :h_c_d_id, :h_c_w_id, :h_d_id, :h_w_id, NOW(), :h_amount, :h_data)

--+ workload_tx_order_status
--= get_customer_by_id
SELECT c_balance, c_first, c_middle, c_last, c_id FROM customer WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id
--= count_customers_by_name
/* TPC-C 2.6.1.2: 60% of Order-Status lookups are by (w_id, d_id, c_last). */
SELECT COUNT(*) FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
--= get_customer_by_name
/* TPC-C 2.6.2.2: pick row ceil(n/2) ordered by c_first — zero-indexed
   OFFSET is (n - 1) / 2, computed client-side. */
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
SELECT MIN(no_o_id) FROM new_order WHERE no_d_id = :d_id AND no_w_id = :w_id
--= delete_new_order
DELETE FROM new_order WHERE no_o_id = :o_id AND no_d_id = :d_id AND no_w_id = :w_id
--= get_order
SELECT o_c_id FROM orders WHERE o_id = :o_id AND o_d_id = :d_id AND o_w_id = :w_id
--= update_order
UPDATE orders SET o_carrier_id = :carrier_id WHERE o_id = :o_id AND o_d_id = :d_id AND o_w_id = :w_id
--= update_order_line
UPDATE order_line SET ol_delivery_d = NOW() WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= get_order_line_amount
SELECT SUM(ol_amount) FROM order_line WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= update_customer
UPDATE customer SET c_balance = c_balance + :amount, c_delivery_cnt = c_delivery_cnt + 1 WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id

--+ workload_tx_stock_level
--= get_district
SELECT d_next_o_id FROM district WHERE d_w_id = :w_id AND d_id = :d_id
--= get_window_items
-- Two-step stock_level scan — see pg.sql for the rationale. Same shape on
-- every dialect so the Go transaction body has no driver-specific branch.
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
