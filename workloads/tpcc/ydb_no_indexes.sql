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
    w_id Int64 NOT NULL,
    w_name Utf8,
    w_street_1 Utf8,
    w_street_2 Utf8,
    w_city Utf8,
    w_state Utf8,
    w_zip Utf8,
    w_tax Double,
    w_ytd Double,
    PRIMARY KEY (w_id)
)
--= district
CREATE TABLE district (
    d_w_id Int64 NOT NULL,
    d_id Int64 NOT NULL,
    d_name Utf8,
    d_street_1 Utf8,
    d_street_2 Utf8,
    d_city Utf8,
    d_state Utf8,
    d_zip Utf8,
    d_tax Double,
    d_ytd Double,
    d_next_o_id Int64,
    PRIMARY KEY (d_w_id, d_id)
)
--= customer
CREATE TABLE customer (
    c_w_id Int64 NOT NULL,
    c_d_id Int64 NOT NULL,
    c_id Int64 NOT NULL,
    c_first Utf8,
    c_middle Utf8,
    c_last Utf8,
    c_street_1 Utf8,
    c_street_2 Utf8,
    c_city Utf8,
    c_state Utf8,
    c_zip Utf8,
    c_phone Utf8,
    c_since Timestamp,
    c_credit Utf8,
    c_credit_lim Double,
    c_discount Double,
    c_balance Double,
    c_ytd_payment Double,
    c_payment_cnt Int64,
    c_delivery_cnt Int64,
    c_data Utf8,
    PRIMARY KEY (c_w_id, c_d_id, c_id)
)
--= history
CREATE TABLE history (
    h_id Int64 NOT NULL,
    h_c_id Int64,
    h_c_d_id Int64,
    h_c_w_id Int64,
    h_d_id Int64,
    h_w_id Int64,
    h_date Timestamp,
    h_amount Double,
    h_data Utf8,
    PRIMARY KEY (h_id)
)
--= new_order
CREATE TABLE new_order (
    no_w_id Int64 NOT NULL,
    no_d_id Int64 NOT NULL,
    no_o_id Int64 NOT NULL,
    PRIMARY KEY (no_w_id, no_d_id, no_o_id)
)
--= orders
CREATE TABLE orders (
    o_w_id Int64 NOT NULL,
    o_d_id Int64 NOT NULL,
    o_id Int64 NOT NULL,
    o_c_id Int64,
    o_entry_d Timestamp,
    o_carrier_id Int64,
    o_ol_cnt Int64,
    o_all_local Int64,
    PRIMARY KEY (o_w_id, o_d_id, o_id)
)
--= order_line
CREATE TABLE order_line (
    ol_w_id Int64 NOT NULL,
    ol_d_id Int64 NOT NULL,
    ol_o_id Int64 NOT NULL,
    ol_number Int64 NOT NULL,
    ol_i_id Int64,
    ol_supply_w_id Int64,
    ol_delivery_d Timestamp,
    ol_quantity Int64,
    ol_amount Double,
    ol_dist_info Utf8,
    PRIMARY KEY (ol_w_id, ol_d_id, ol_o_id, ol_number)
)
--= item
CREATE TABLE item (
    i_id Int64 NOT NULL,
    i_im_id Int64,
    i_name Utf8,
    i_price Double,
    i_data Utf8,
    PRIMARY KEY (i_id)
)
--= stock
CREATE TABLE stock (
    s_w_id Int64 NOT NULL,
    s_i_id Int64 NOT NULL,
    s_quantity Int64,
    s_dist_01 Utf8,
    s_dist_02 Utf8,
    s_dist_03 Utf8,
    s_dist_04 Utf8,
    s_dist_05 Utf8,
    s_dist_06 Utf8,
    s_dist_07 Utf8,
    s_dist_08 Utf8,
    s_dist_09 Utf8,
    s_dist_10 Utf8,
    s_ytd Int64,
    s_order_cnt Int64,
    s_remote_cnt Int64,
    s_data Utf8,
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
UPSERT INTO orders (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)
VALUES (:o_id, :d_id, :w_id, :c_id, CurrentUtcTimestamp(), :ol_cnt, :all_local)
--= insert_new_order
UPSERT INTO new_order (no_o_id, no_d_id, no_w_id) VALUES (:o_id, :d_id, :w_id)
--= get_item
SELECT i_price, i_name, i_data FROM item WHERE i_id = :i_id
--= get_stock
SELECT s_quantity, s_data, s_dist_01, s_dist_02, s_dist_03, s_dist_04, s_dist_05, s_dist_06, s_dist_07, s_dist_08, s_dist_09, s_dist_10
FROM stock WHERE s_i_id = :i_id AND s_w_id = :w_id
--= update_stock
UPDATE stock SET s_quantity = :quantity, s_ytd = s_ytd + :ol_quantity, s_order_cnt = s_order_cnt + 1, s_remote_cnt = s_remote_cnt + :remote_cnt
WHERE s_i_id = :i_id AND s_w_id = :w_id
--= insert_order_line
UPSERT INTO order_line (ol_o_id, ol_d_id, ol_w_id, ol_number, ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_dist_info)
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
/* Trailing c_data is needed for the §2.5.2.2 BC-credit append path. */
SELECT c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since, c_data
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= count_customers_by_name
/* TPC-C 2.5.1.2: 60% of Payment lookups are by (w_id, d_id, c_last). */
SELECT COUNT(*) FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
--= get_customer_by_name
/* TPC-C 2.5.2.2: pick row ceil(n/2) ordered by c_first — zero-indexed
   OFFSET is (n - 1) / 2, computed client-side and passed in.
   Trailing c_data supports the BC-credit append path (§1.8).
   Note: YDB OFFSET requires Uint64; JS Number arrives as Int64 via
   AutoDeclare, so wrap in CAST to satisfy the type checker. */
SELECT c_id, c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since, c_data
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
ORDER BY c_first
LIMIT 1 OFFSET CAST(:offset AS Uint64)
--= update_customer
UPDATE customer SET c_balance = c_balance - :amount, c_ytd_payment = c_ytd_payment + :amount, c_payment_cnt = c_payment_cnt + 1
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= update_customer_bc
/* TPC-C 2.5.2.2: BC-credit path. c_data_new is built AND clamped to
   500 chars on the JS side, so this UPDATE just assigns it raw —
   sidesteps YDB's Substring(String) vs Utf8 type mismatch. */
UPDATE customer
   SET c_balance     = c_balance - :amount,
       c_ytd_payment = c_ytd_payment + :amount,
       c_payment_cnt = c_payment_cnt + 1,
       c_data        = :c_data_new
 WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= insert_history
UPSERT INTO history (h_id, h_c_id, h_c_d_id, h_c_w_id, h_d_id, h_w_id, h_date, h_amount, h_data)
VALUES (:h_id, :h_c_id, :h_c_d_id, :h_c_w_id, :h_d_id, :h_w_id, CurrentUtcTimestamp(), :h_amount, :h_data)

--+ workload_tx_order_status
--= get_customer_by_id
SELECT c_balance, c_first, c_middle, c_last, c_id FROM customer WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id
--= count_customers_by_name
/* TPC-C 2.6.1.2: 60% of Order-Status lookups are by (w_id, d_id, c_last). */
SELECT COUNT(*) FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
--= get_customer_by_name
/* TPC-C 2.6.2.2: pick row ceil(n/2) ordered by c_first — zero-indexed
   OFFSET is (n - 1) / 2, computed client-side.
   Note: YDB OFFSET requires Uint64; CAST forces the type. */
SELECT c_balance, c_first, c_middle, c_last, c_id FROM customer
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_last = :c_last
ORDER BY c_first
LIMIT 1 OFFSET CAST(:offset AS Uint64)
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
UPDATE order_line SET ol_delivery_d = CurrentUtcTimestamp() WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= get_order_line_amount
SELECT SUM(ol_amount) FROM order_line WHERE ol_o_id = :o_id AND ol_d_id = :d_id AND ol_w_id = :w_id
--= update_customer
UPDATE customer SET c_balance = c_balance + :amount, c_delivery_cnt = c_delivery_cnt + 1 WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id

--+ workload_tx_stock_level
--= get_district
SELECT d_next_o_id FROM district WHERE d_w_id = :w_id AND d_id = :d_id
--= get_window_items
-- Two-step stock_level scan — see pg.sql for the rationale.
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
