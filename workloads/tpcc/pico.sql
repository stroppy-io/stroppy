--+ drop_schema
--= query
DROP TABLE IF EXISTS order_line;
--= query
DROP TABLE IF EXISTS new_order;
--= query
DROP TABLE IF EXISTS orders;
--= query
DROP TABLE IF EXISTS history;
--= query
DROP TABLE IF EXISTS stock;
--= query
DROP TABLE IF EXISTS customer;
--= query
DROP TABLE IF EXISTS district;
--= query
DROP TABLE IF EXISTS warehouse;
--= query
DROP TABLE IF EXISTS item;

--+ create_schema
--= query
CREATE TABLE warehouse (
  w_id INTEGER PRIMARY KEY,
  w_name TEXT,
  w_street_1 TEXT,
  w_street_2 TEXT,
  w_city TEXT,
  w_state TEXT,
  w_zip TEXT,
  w_tax DECIMAL(4,4),
  w_ytd DECIMAL(12,2)
)
--= query
CREATE TABLE district (
  d_id INTEGER,
  d_w_id INTEGER,
  d_name TEXT,
  d_street_1 TEXT,
  d_street_2 TEXT,
  d_city TEXT,
  d_state TEXT,
  d_zip TEXT,
  d_tax DECIMAL(4,4),
  d_ytd DECIMAL(12,2),
  d_next_o_id INTEGER,
  PRIMARY KEY (d_w_id, d_id)
)
--= query
CREATE TABLE customer (
  c_id INTEGER,
  c_d_id INTEGER,
  c_w_id INTEGER,
  c_first TEXT,
  c_middle TEXT,
  c_last TEXT,
  c_street_1 TEXT,
  c_street_2 TEXT,
  c_city TEXT,
  c_state TEXT,
  c_zip TEXT,
  c_phone TEXT,
  c_since DATETIME,
  c_credit TEXT,
  c_credit_lim DECIMAL(12,2),
  c_discount DECIMAL(4,4),
  c_balance DECIMAL(12,2),
  c_ytd_payment DECIMAL(12,2),
  c_payment_cnt INTEGER,
  c_delivery_cnt INTEGER,
  c_data TEXT,
  PRIMARY KEY (c_w_id, c_d_id, c_id)
)
--= query
CREATE TABLE history (
  h_date_id DATETIME PRIMARY KEY,
  h_c_id INTEGER,
  h_c_d_id INTEGER,
  h_c_w_id INTEGER,
  h_d_id INTEGER,
  h_w_id INTEGER,
  h_date DATETIME,
  h_amount DECIMAL(6,2),
  h_data TEXT
)
--= query
CREATE TABLE new_order (
  date_id DATETIME,
  no_o_id INTEGER,
  no_d_id INTEGER,
  no_w_id INTEGER,
  PRIMARY KEY (no_w_id, no_d_id, no_o_id, date_id)
)
--= query
CREATE TABLE orders (
  date_id DATETIME,
  o_id INTEGER,
  o_d_id INTEGER,
  o_w_id INTEGER,
  o_c_id INTEGER,
  o_entry_d DATETIME,
  o_carrier_id INTEGER,
  o_ol_cnt INTEGER,
  o_all_local INTEGER,
  PRIMARY KEY (o_w_id, o_d_id, o_id, date_id)
)
--= query
CREATE TABLE order_line (
  ol_o_id INTEGER,
  ol_d_id INTEGER,
  ol_w_id INTEGER,
  ol_number INTEGER,
  ol_i_id INTEGER,
  ol_supply_w_id INTEGER,
  ol_delivery_d DATETIME,
  ol_quantity INTEGER,
  ol_amount DECIMAL(6,2),
  ol_dist_info TEXT,
  PRIMARY KEY (ol_w_id, ol_d_id, ol_o_id, ol_number)
)
--= query
CREATE TABLE item (
  i_id INTEGER PRIMARY KEY,
  i_im_id INTEGER,
  i_name TEXT,
  i_price DECIMAL(5,2),
  i_data TEXT
)
--= query
CREATE TABLE stock (
  s_i_id INTEGER,
  s_w_id INTEGER,
  s_quantity INTEGER,
  s_dist_01 TEXT,
  s_dist_02 TEXT,
  s_dist_03 TEXT,
  s_dist_04 TEXT,
  s_dist_05 TEXT,
  s_dist_06 TEXT,
  s_dist_07 TEXT,
  s_dist_08 TEXT,
  s_dist_09 TEXT,
  s_dist_10 TEXT,
  s_ytd INTEGER,
  s_order_cnt INTEGER,
  s_remote_cnt INTEGER,
  s_data TEXT,
  PRIMARY KEY (s_i_id, s_w_id)
)

--+ workload
--= neword_get_customer_warehouse
SELECT c.c_discount, c.c_last, c.c_credit, w.w_tax
FROM customer c
JOIN warehouse w ON w.w_id = c.c_w_id
WHERE c.c_w_id = :w_id AND c.c_d_id = :d_id AND c.c_id = :c_id
--= neword_get_district
SELECT d_next_o_id, d_tax FROM district WHERE d_id = :d_id AND d_w_id = :w_id
--= neword_update_district
UPDATE district SET d_next_o_id = d_next_o_id + 1 WHERE d_id = :d_id AND d_w_id = :w_id
--= neword_insert_order
INSERT INTO orders (date_id, o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)
VALUES (current_timestamp, :o_id, :d_id, :w_id, :c_id, current_timestamp, :ol_cnt, :all_local)
--= neword_insert_new_order
INSERT INTO new_order (no_o_id, no_d_id, no_w_id, date_id) VALUES (:o_id, :d_id, :w_id, current_timestamp)
--= neword_get_item
SELECT i_price, i_name, i_data FROM item WHERE i_id = :i_id
--= neword_get_stock
SELECT s_quantity, s_data, s_dist_01, s_dist_02, s_dist_03, s_dist_04, s_dist_05, s_dist_06, s_dist_07, s_dist_08, s_dist_09, s_dist_10
FROM stock WHERE s_i_id = :i_id AND s_w_id = :w_id
--= neword_update_stock
UPDATE stock SET s_quantity = :quantity, s_ytd = s_ytd + :ol_quantity, s_order_cnt = s_order_cnt + 1, s_remote_cnt = s_remote_cnt + :remote_cnt
WHERE s_i_id = :i_id AND s_w_id = :w_id
--= neword_insert_order_line
INSERT INTO order_line (ol_o_id, ol_d_id, ol_w_id, ol_number, ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_dist_info)
VALUES (:o_id, :d_id, :w_id, :ol_number, :i_id, :supply_w_id, :quantity, :amount, :dist_info)

--= payment_update_warehouse
UPDATE warehouse SET w_ytd = w_ytd + :amount WHERE w_id = :w_id
--= payment_get_warehouse
SELECT w_name, w_street_1, w_street_2, w_city, w_state, w_zip FROM warehouse WHERE w_id = :w_id
--= payment_update_district
UPDATE district SET d_ytd = d_ytd + :amount WHERE d_w_id = :w_id AND d_id = :d_id
--= payment_get_district
SELECT d_name, d_street_1, d_street_2, d_city, d_state, d_zip FROM district WHERE d_w_id = :w_id AND d_id = :d_id
--= payment_count_customer_by_name
SELECT count(c_last) FROM customer WHERE c_last = :c_last AND c_d_id = :d_id AND c_w_id = :w_id
--= payment_get_customer_by_name
SELECT c_id, c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since
FROM customer WHERE c_last = :c_last AND c_d_id = :d_id AND c_w_id = :w_id ORDER BY c_first
--= payment_get_customer_by_id
SELECT c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= payment_update_customer
UPDATE customer SET c_balance = c_balance - :amount, c_ytd_payment = c_ytd_payment + :amount, c_payment_cnt = c_payment_cnt + 1
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= payment_update_customer_bc
UPDATE customer SET c_balance = c_balance - :amount, c_ytd_payment = c_ytd_payment + :amount, c_payment_cnt = c_payment_cnt + 1, c_data = :c_data
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= payment_insert_history
INSERT INTO history (h_date_id, h_c_id, h_c_d_id, h_c_w_id, h_d_id, h_w_id, h_date, h_amount, h_data)
VALUES (current_timestamp, :h_c_id, :h_c_d_id, :h_c_w_id, :h_d_id, :h_w_id, current_timestamp, :h_amount, :h_data)

--= ostat_count_customer_by_name
SELECT count(c_id) FROM customer WHERE c_last = :c_last AND c_d_id = :d_id AND c_w_id = :w_id
--= ostat_get_customer_by_name
SELECT c_balance, c_first, c_middle, c_id FROM customer WHERE c_last = :c_last AND c_d_id = :d_id AND c_w_id = :w_id ORDER BY c_first
--= ostat_get_customer_by_id
SELECT c_balance, c_first, c_middle, c_last FROM customer WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id
--= ostat_get_last_order
SELECT o_id, o_carrier_id, o_entry_d FROM orders WHERE o_d_id = :d_id AND o_w_id = :w_id AND o_c_id = :c_id ORDER BY o_id DESC LIMIT 1
--= ostat_get_order_lines
SELECT ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_delivery_d FROM order_line WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id

--= delivery_get_min_new_order
SELECT min(no_o_id) FROM new_order WHERE no_d_id = :d_id AND no_w_id = :w_id
--= delivery_delete_new_order
DELETE FROM new_order WHERE no_o_id = :o_id AND no_d_id = :d_id AND no_w_id = :w_id
--= delivery_get_order
SELECT o_c_id FROM orders WHERE o_id = :o_id AND o_d_id = :d_id AND o_w_id = :w_id
--= delivery_update_order
UPDATE orders SET o_carrier_id = :carrier_id WHERE o_id = :o_id AND o_d_id = :d_id AND o_w_id = :w_id
--= delivery_update_order_line
UPDATE order_line SET ol_delivery_d = current_timestamp WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= delivery_get_order_line_amount
SELECT SUM(ol_amount) FROM order_line WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= delivery_update_customer
UPDATE customer SET c_balance = c_balance + :amount, c_delivery_cnt = c_delivery_cnt + 1 WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id

--= slev_get_district
SELECT d_next_o_id FROM district WHERE d_w_id = :w_id AND d_id = :d_id
--= slev_stock_count
WITH cte1 AS (
  SELECT DISTINCT ol_i_id
  FROM order_line
  WHERE ol_w_id = :w_id 
    AND ol_d_id = :d_id 
    AND ol_o_id >= :min_o_id 
    AND ol_o_id < :next_o_id
)
SELECT COUNT(*)
FROM cte1
JOIN stock 
  ON s_i_id = ol_i_id 
  AND s_w_id = :w_id
WHERE s_quantity < :threshold