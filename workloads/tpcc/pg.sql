--+ drop_schema
--= drop functions
DROP FUNCTION IF EXISTS SLEV, OSTAT, DELIVERY, PAYMENT, NEWORD, DBMS_RANDOM;
--= drop tables
DROP TABLE IF EXISTS order_line, new_order, orders, history, stock, customer, district, warehouse, item CASCADE;

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
  d_w_id INTEGER REFERENCES warehouse(w_id),
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
  c_w_id INTEGER REFERENCES warehouse(w_id),
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
  no_w_id INTEGER REFERENCES warehouse(w_id),
  PRIMARY KEY (no_w_id, no_d_id, no_o_id)
)
--= orders
CREATE TABLE orders (
  o_id INTEGER,
  o_d_id INTEGER,
  o_w_id INTEGER REFERENCES warehouse(w_id),
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
  ol_w_id INTEGER REFERENCES warehouse(w_id),
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
  s_i_id INTEGER REFERENCES item(i_id),
  s_w_id INTEGER REFERENCES warehouse(w_id),
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

--+ create_procedures
--= neword
CREATE OR REPLACE FUNCTION NEWORD (
  no_w_id INTEGER,
  no_max_w_id INTEGER,
  no_d_id INTEGER,
  no_c_id INTEGER,
  no_o_ol_cnt INTEGER
) RETURNS INTEGER AS $$
DECLARE
  no_c_discount NUMERIC;
  no_c_last VARCHAR;
  no_c_credit VARCHAR;
  no_d_tax NUMERIC;
  no_w_tax NUMERIC;
  no_d_next_o_id INTEGER;
  no_o_all_local INTEGER;
  v_i_id INTEGER;
  v_supply_w_id INTEGER;
  v_quantity INTEGER;
  v_s_quantity INTEGER;
  v_i_price NUMERIC;
  v_i_name VARCHAR;
  v_i_data VARCHAR;
  v_s_data VARCHAR;
  v_dist_info CHAR(24);
  v_amount NUMERIC;
  loop_counter INTEGER;
BEGIN
  no_o_all_local := 1;

  SELECT c_discount, c_last, c_credit
    INTO no_c_discount, no_c_last, no_c_credit
  FROM customer
  WHERE c_w_id = no_w_id AND c_d_id = no_d_id AND c_id = no_c_id;

  SELECT w_tax INTO no_w_tax FROM warehouse WHERE w_id = no_w_id;

  UPDATE district SET d_next_o_id = d_next_o_id + 1
  WHERE d_id = no_d_id AND d_w_id = no_w_id
  RETURNING d_next_o_id - 1, d_tax INTO no_d_next_o_id, no_d_tax;

  INSERT INTO orders (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)
  VALUES (no_d_next_o_id, no_d_id, no_w_id, no_c_id, current_timestamp, no_o_ol_cnt, no_o_all_local);

  INSERT INTO new_order (no_o_id, no_d_id, no_w_id)
  VALUES (no_d_next_o_id, no_d_id, no_w_id);

  FOR loop_counter IN 1 .. no_o_ol_cnt
  LOOP
    v_i_id := 1 + (floor(random() * 100000))::INTEGER;
    v_supply_w_id := no_w_id;
    v_quantity := 1 + (floor(random() * 10))::INTEGER;

    SELECT i_price, i_name, i_data INTO v_i_price, v_i_name, v_i_data
    FROM item WHERE i_id = v_i_id;

    IF NOT FOUND THEN
      CONTINUE;
    END IF;

    SELECT s_quantity, s_data,
      CASE no_d_id
        WHEN 1 THEN s_dist_01
        WHEN 2 THEN s_dist_02
        WHEN 3 THEN s_dist_03
        WHEN 4 THEN s_dist_04
        WHEN 5 THEN s_dist_05
        WHEN 6 THEN s_dist_06
        WHEN 7 THEN s_dist_07
        WHEN 8 THEN s_dist_08
        WHEN 9 THEN s_dist_09
        WHEN 10 THEN s_dist_10
      END
    INTO v_s_quantity, v_s_data, v_dist_info
    FROM stock
    WHERE s_i_id = v_i_id AND s_w_id = v_supply_w_id;

    IF v_s_quantity - v_quantity >= 10 THEN
      v_s_quantity := v_s_quantity - v_quantity;
    ELSE
      v_s_quantity := v_s_quantity - v_quantity + 91;
    END IF;

    UPDATE stock
      SET s_quantity = v_s_quantity,
          s_ytd = s_ytd + v_quantity,
          s_order_cnt = s_order_cnt + 1,
          s_remote_cnt = s_remote_cnt + 0
    WHERE s_i_id = v_i_id AND s_w_id = v_supply_w_id;

    v_amount := v_quantity * v_i_price;

    INSERT INTO order_line
      (ol_o_id, ol_d_id, ol_w_id, ol_number, ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_dist_info)
    VALUES
      (no_d_next_o_id, no_d_id, no_w_id, loop_counter, v_i_id, v_supply_w_id, v_quantity, v_amount, v_dist_info);
  END LOOP;

  RETURN no_d_next_o_id;
END;
$$ LANGUAGE 'plpgsql';

--= payment
CREATE OR REPLACE FUNCTION PAYMENT (
  p_w_id INTEGER,
  p_d_id INTEGER,
  p_c_w_id INTEGER,
  p_c_d_id INTEGER,
  p_c_id_in INTEGER,
  byname INTEGER,
  p_h_amount NUMERIC,
  p_c_last_in VARCHAR,
  p_h_id BIGINT
) RETURNS INTEGER AS $$
DECLARE
  p_c_balance NUMERIC(12, 2);
  p_c_credit CHAR(2);
  p_c_last VARCHAR(16);
  p_c_id INTEGER;
  p_w_name VARCHAR(11);
  p_d_name VARCHAR(11);
  name_count INTEGER;
  h_data_val VARCHAR(30);
BEGIN
  p_c_id := p_c_id_in;
  p_c_last := p_c_last_in;

  UPDATE warehouse
  SET w_ytd = w_ytd + p_h_amount
  WHERE w_id = p_w_id
  RETURNING w_name INTO p_w_name;

  UPDATE district
  SET d_ytd = d_ytd + p_h_amount
  WHERE d_w_id = p_w_id AND d_id = p_d_id
  RETURNING d_name INTO p_d_name;

  IF ( byname = 1 ) THEN
    SELECT count(c_last) INTO name_count
    FROM customer
    WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id;

    IF name_count > 0 THEN
      SELECT c_id, c_balance, c_credit
      INTO p_c_id, p_c_balance, p_c_credit
      FROM customer
      WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id
      ORDER BY c_first
      LIMIT 1 OFFSET (name_count / 2);
    END IF;
  ELSE
    SELECT c_balance, c_credit
    INTO p_c_balance, p_c_credit
    FROM customer
    WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;
  END IF;

  h_data_val := COALESCE(p_w_name,'') || ' ' || COALESCE(p_d_name,'');

  UPDATE customer
  SET c_balance = c_balance - p_h_amount,
      c_ytd_payment = c_ytd_payment + p_h_amount,
      c_payment_cnt = c_payment_cnt + 1
  WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;

  INSERT INTO history (h_id, h_c_d_id, h_c_w_id, h_c_id, h_d_id, h_w_id, h_date, h_amount, h_data)
  VALUES (p_h_id, p_c_d_id, p_c_w_id, p_c_id, p_d_id, p_w_id, current_timestamp, p_h_amount, h_data_val);

  RETURN p_c_id;
END;
$$ LANGUAGE 'plpgsql';

--= delivery
CREATE OR REPLACE FUNCTION DELIVERY (
  d_w_id INTEGER,
  d_o_carrier_id INTEGER
) RETURNS INTEGER AS $$
DECLARE
  v_d_id INTEGER;
  v_no_o_id INTEGER;
  v_c_id INTEGER;
  v_ol_total NUMERIC;
BEGIN
  FOR v_d_id IN 1 .. 10 LOOP
    SELECT min(no_o_id) INTO v_no_o_id
    FROM new_order
    WHERE no_d_id = v_d_id AND no_w_id = d_w_id;

    IF v_no_o_id IS NULL THEN
      CONTINUE;
    END IF;

    DELETE FROM new_order
    WHERE no_o_id = v_no_o_id AND no_d_id = v_d_id AND no_w_id = d_w_id;

    SELECT o_c_id INTO v_c_id
    FROM orders
    WHERE o_id = v_no_o_id AND o_d_id = v_d_id AND o_w_id = d_w_id;

    UPDATE orders
    SET o_carrier_id = d_o_carrier_id
    WHERE o_id = v_no_o_id AND o_d_id = v_d_id AND o_w_id = d_w_id;

    UPDATE order_line
    SET ol_delivery_d = current_timestamp
    WHERE ol_o_id = v_no_o_id AND ol_d_id = v_d_id AND ol_w_id = d_w_id;

    SELECT COALESCE(SUM(ol_amount), 0) INTO v_ol_total
    FROM order_line
    WHERE ol_o_id = v_no_o_id AND ol_d_id = v_d_id AND ol_w_id = d_w_id;

    UPDATE customer
    SET c_balance = c_balance + v_ol_total,
        c_delivery_cnt = c_delivery_cnt + 1
    WHERE c_id = v_c_id AND c_d_id = v_d_id AND c_w_id = d_w_id;
  END LOOP;

  RETURN 1;
END;
$$ LANGUAGE 'plpgsql';

--= ostat
CREATE OR REPLACE FUNCTION OSTAT (
  os_w_id INTEGER,
  os_d_id INTEGER,
  os_c_id INTEGER,
  byname INTEGER,
  os_c_last VARCHAR
) RETURNS INTEGER AS $$
DECLARE
  namecnt INTEGER;
  v_c_id INTEGER;
  v_c_balance NUMERIC;
  v_c_first VARCHAR;
  v_c_middle VARCHAR;
  v_o_id INTEGER;
  v_entdate TIMESTAMP;
  v_o_carrier_id INTEGER;
BEGIN
  v_c_id := os_c_id;

  IF ( byname = 1 ) THEN
    SELECT count(c_id) INTO namecnt
    FROM customer
    WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id;

    IF namecnt > 0 THEN
      SELECT c_balance, c_first, c_middle, c_id
      INTO v_c_balance, v_c_first, v_c_middle, v_c_id
      FROM customer
      WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id
      ORDER BY c_first
      LIMIT 1 OFFSET ((namecnt + 1) / 2);
    END IF;
  ELSE
    SELECT c_balance, c_first, c_middle
    INTO v_c_balance, v_c_first, v_c_middle
    FROM customer
    WHERE c_id = v_c_id AND c_d_id = os_d_id AND c_w_id = os_w_id;
  END IF;

  SELECT o_id, o_carrier_id, o_entry_d
  INTO v_o_id, v_o_carrier_id, v_entdate
  FROM orders
  WHERE o_d_id = os_d_id AND o_w_id = os_w_id AND o_c_id = v_c_id
  ORDER BY o_id DESC
  LIMIT 1;

  RETURN COALESCE(v_o_id, 0);
END;
$$ LANGUAGE 'plpgsql';

--= slev
CREATE OR REPLACE FUNCTION SLEV (
  st_w_id INTEGER,
  st_d_id INTEGER,
  threshold INTEGER
) RETURNS INTEGER AS $$
DECLARE
  stock_count INTEGER;
  v_next_o_id INTEGER;
BEGIN
  SELECT d_next_o_id INTO v_next_o_id
  FROM district
  WHERE d_w_id = st_w_id AND d_id = st_d_id;

  SELECT COUNT(DISTINCT s_i_id) INTO stock_count
  FROM order_line, stock
  WHERE ol_w_id = st_w_id
    AND ol_d_id = st_d_id
    AND ol_o_id < v_next_o_id
    AND ol_o_id >= (v_next_o_id - 20)
    AND s_w_id = st_w_id
    AND s_i_id = ol_i_id
    AND s_quantity < threshold;

  RETURN COALESCE(stock_count, 0);
END;
$$ LANGUAGE 'plpgsql';

--+ workload_procs
--= new_order
SELECT NEWORD(:w_id, :max_w_id, :d_id, :c_id, :ol_cnt)
--= payment
SELECT PAYMENT(:p_w_id, :p_d_id, :p_c_w_id, :p_c_d_id, :p_c_id, :byname, :h_amount, :c_last, :p_h_id)
--= order_status
SELECT OSTAT(:os_w_id, :os_d_id, :os_c_id, :byname, :os_c_last)
--= delivery
SELECT DELIVERY(:d_w_id, :d_o_carrier_id)
--= stock_level
SELECT SLEV(:st_w_id, :st_d_id, :threshold)

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
SELECT c_first, c_middle, c_last, c_street_1, c_street_2, c_city, c_state, c_zip, c_phone, c_credit, c_credit_lim, c_discount, c_balance, c_since
FROM customer WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= update_customer
UPDATE customer SET c_balance = c_balance - :amount, c_ytd_payment = c_ytd_payment + :amount, c_payment_cnt = c_payment_cnt + 1
WHERE c_w_id = :w_id AND c_d_id = :d_id AND c_id = :c_id
--= insert_history
INSERT INTO history (h_id, h_c_id, h_c_d_id, h_c_w_id, h_d_id, h_w_id, h_date, h_amount, h_data)
VALUES (:h_id, :h_c_id, :h_c_d_id, :h_c_w_id, :h_d_id, :h_w_id, current_timestamp, :h_amount, :h_data)

--+ workload_tx_order_status
--= get_customer_by_id
SELECT c_balance, c_first, c_middle, c_last FROM customer WHERE c_id = :c_id AND c_d_id = :d_id AND c_w_id = :w_id
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
-- Step 1 of stock_level: collect distinct item ids from the last-20-orders
-- window. The same two-step shape runs against all four dialects — the
-- single-query JOIN form trips picodata's sbroad planner intermittently,
-- and a uniform script is worth the extra query here (stock_level is 4%
-- of the mix).
SELECT DISTINCT ol_i_id FROM order_line
WHERE ol_w_id = :w_id
  AND ol_d_id = :d_id
  AND ol_o_id >= :min_o_id
  AND ol_o_id < :next_o_id
--= stock_count_in
-- Step 2: count low-stock items. The {ids} placeholder is replaced in
-- TypeScript with an integer list built from step 1's result — stroppy's
-- :name substitution doesn't touch IN list contents.
SELECT COUNT(*) FROM stock
WHERE s_w_id = :w_id
  AND s_quantity < :threshold
  AND s_i_id IN ({ids})
