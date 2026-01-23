--+ drop_schema
--= drop functions
DROP FUNCTION IF EXISTS SLEV, OSTAT, DELIVERY, PAYMENT, NEWORD, DBMS_RANDOM;
--= drop tables
DROP TABLE IF EXISTS order_line, new_order, orders, history, stock, customer, district, warehouse, item CASCADE;

--+ create_schema
--= query
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
--= query
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
--= query
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
--= query
CREATE TABLE history (
  h_c_id INTEGER,
  h_c_d_id INTEGER,
  h_c_w_id INTEGER,
  h_d_id INTEGER,
  h_w_id INTEGER,
  h_date TIMESTAMP,
  h_amount DECIMAL(6,2),
  h_data VARCHAR(24)
)
--= query
CREATE TABLE new_order (
  no_o_id INTEGER,
  no_d_id INTEGER,
  no_w_id INTEGER REFERENCES warehouse(w_id),
  PRIMARY KEY (no_w_id, no_d_id, no_o_id)
)
--= query
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
--= query
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
--= query
CREATE TABLE item (
  i_id INTEGER PRIMARY KEY,
  i_im_id INTEGER,
  i_name VARCHAR(24),
  i_price DECIMAL(5,2),
  i_data VARCHAR(50)
)
--= query
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

--= query
CREATE OR REPLACE FUNCTION DBMS_RANDOM (INTEGER, INTEGER) RETURNS INTEGER AS $$
DECLARE
  start_int ALIAS FOR $1;
  end_int ALIAS FOR $2;
BEGIN
  RETURN trunc(random() * (end_int-start_int + 1) + start_int);
END;
$$ LANGUAGE 'plpgsql' STRICT;

--= query
CREATE OR REPLACE FUNCTION NEWORD (
  no_w_id INTEGER,
  no_max_w_id INTEGER,
  no_d_id INTEGER,
  no_c_id INTEGER,
  no_o_ol_cnt INTEGER,
  no_d_next_o_id INTEGER
) RETURNS NUMERIC AS $$
DECLARE
  no_c_discount NUMERIC;
  no_c_last VARCHAR;
  no_c_credit VARCHAR;
  no_d_tax NUMERIC;
  no_w_tax NUMERIC;
  no_s_quantity NUMERIC;
  no_o_all_local SMALLINT;
  rbk SMALLINT;
  item_id_array INT[];
  supply_wid_array INT[];
  quantity_array SMALLINT[];
  order_line_array SMALLINT[];
  stock_dist_array CHAR(24)[];
  s_quantity_array SMALLINT[];
  price_array NUMERIC(5,2)[];
  amount_array NUMERIC(5,2)[];
BEGIN
  no_o_all_local := 1;
  SELECT c_discount, c_last, c_credit, w_tax
  INTO no_c_discount, no_c_last, no_c_credit, no_w_tax
  FROM customer, warehouse
  WHERE warehouse.w_id = no_w_id AND customer.c_w_id = no_w_id
    AND customer.c_d_id = no_d_id AND customer.c_id = no_c_id;

  --#2.4.1.4
  rbk := round(DBMS_RANDOM(1,100));

  --#2.4.1.5
  FOR loop_counter IN 1 .. no_o_ol_cnt
  LOOP
    IF ((loop_counter = no_o_ol_cnt) AND (rbk = 1))
    THEN
      item_id_array[loop_counter] := 100001;
    ELSE
      item_id_array[loop_counter] := round(DBMS_RANDOM(1,100000));
    END IF;

    --#2.4.1.5.2
    IF ( round(DBMS_RANDOM(1,100)) > 1 )
    THEN
      supply_wid_array[loop_counter] := no_w_id;
    ELSE
      no_o_all_local := 0;
      supply_wid_array[loop_counter] := 1 + MOD(CAST (no_w_id + round(DBMS_RANDOM(0,no_max_w_id-1)) AS INT), no_max_w_id);
    END IF;

    --#2.4.1.5.3
    quantity_array[loop_counter] := round(DBMS_RANDOM(1,10));
    order_line_array[loop_counter] := loop_counter;
  END LOOP;

  UPDATE district SET d_next_o_id = d_next_o_id + 1
  WHERE d_id = no_d_id AND d_w_id = no_w_id
  RETURNING d_next_o_id - 1, d_tax INTO no_d_next_o_id, no_d_tax;

  INSERT INTO ORDERS (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)
  VALUES (no_d_next_o_id, no_d_id, no_w_id, no_c_id, current_timestamp, no_o_ol_cnt, no_o_all_local);

  INSERT INTO NEW_ORDER (no_o_id, no_d_id, no_w_id)
  VALUES (no_d_next_o_id, no_d_id, no_w_id);

  -- Stock and order line processing (simplified for brevity)
  -- Full implementation would include district-specific s_dist processing

  RETURN no_d_next_o_id;
EXCEPTION
  WHEN serialization_failure OR deadlock_detected OR no_data_found
  THEN ROLLBACK; RETURN -1;
END;
$$ LANGUAGE 'plpgsql';

--= query
CREATE OR REPLACE FUNCTION PAYMENT (
  p_w_id INTEGER,
  p_d_id INTEGER,
  p_c_w_id INTEGER,
  p_c_d_id INTEGER,
  p_c_id_in INTEGER,
  byname INTEGER,
  p_h_amount NUMERIC,
  p_c_last_in VARCHAR
) RETURNS INTEGER AS $$
DECLARE
  p_c_balance NUMERIC(12, 2);
  p_c_credit CHAR(2);
  p_c_last VARCHAR(16);
  p_c_id INTEGER;
  p_w_name VARCHAR(11);
  p_d_name VARCHAR(11);
  name_count SMALLINT;
  h_data VARCHAR(30);
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

  IF ( byname = 1 )
  THEN
    SELECT count(c_last) INTO name_count
    FROM customer
    WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id;

    -- Get middle customer (simplified)
    SELECT c_id, c_balance, c_credit
    INTO p_c_id, p_c_balance, p_c_credit
    FROM customer
    WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id
    ORDER BY c_first
    LIMIT 1 OFFSET (name_count / 2);
  ELSE
    SELECT c_balance, c_credit
    INTO p_c_balance, p_c_credit
    FROM customer
    WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;
  END IF;

  h_data := p_w_name || ' ' || p_d_name;

  -- Update customer balance
  UPDATE customer
  SET c_balance = c_balance - p_h_amount,
      c_ytd_payment = c_ytd_payment + p_h_amount,
      c_payment_cnt = c_payment_cnt + 1
  WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;

  INSERT INTO history (h_c_d_id, h_c_w_id, h_c_id, h_d_id, h_w_id, h_date, h_amount, h_data)
  VALUES (p_c_d_id, p_c_w_id, p_c_id, p_d_id, p_w_id, current_timestamp, p_h_amount, h_data);

  RETURN p_c_id;
EXCEPTION
  WHEN serialization_failure OR deadlock_detected OR no_data_found
  THEN ROLLBACK; RETURN -1;
END;
$$ LANGUAGE 'plpgsql';

--= query
CREATE OR REPLACE FUNCTION DELIVERY (
  d_w_id INTEGER,
  d_o_carrier_id INTEGER
) RETURNS INTEGER AS $$
DECLARE
  loop_counter SMALLINT;
  d_id_in_array SMALLINT[] := ARRAY[1,2,3,4,5,6,7,8,9,10];
  d_id_array SMALLINT[];
  o_id_array INT[];
  c_id_array INT[];
  sum_amounts NUMERIC[];
BEGIN
  -- Delete from new_order and get order IDs
  WITH new_order_delete AS (
    DELETE FROM new_order as del_new_order
    USING UNNEST(d_id_in_array) AS d_ids
    WHERE no_d_id = d_ids
      AND no_w_id = d_w_id
      AND del_new_order.no_o_id = (
        select min(select_new_order.no_o_id)
        from new_order as select_new_order
        where no_d_id = d_ids and no_w_id = d_w_id
      )
    RETURNING del_new_order.no_o_id, del_new_order.no_d_id
  )
  SELECT array_agg(no_o_id), array_agg(no_d_id)
  FROM new_order_delete
  INTO o_id_array, d_id_array;

  -- Update orders with carrier
  UPDATE orders
  SET o_carrier_id = d_o_carrier_id
  FROM UNNEST(o_id_array, d_id_array) AS ids(o_id, d_id)
  WHERE orders.o_id = ids.o_id
    AND o_d_id = ids.d_id
    AND o_w_id = d_w_id;

  -- Update order lines and get amounts
  WITH order_line_update AS (
    UPDATE order_line
    SET ol_delivery_d = current_timestamp
    FROM UNNEST(o_id_array, d_id_array) AS ids(o_id, d_id)
    WHERE ol_o_id = ids.o_id
      AND ol_d_id = ids.d_id
      AND ol_w_id = d_w_id
    RETURNING ol_d_id, ol_o_id, ol_amount
  )
  SELECT array_agg(ol_d_id), array_agg(c_id), array_agg(sum_amount)
  FROM (
    SELECT ol_d_id,
           (SELECT DISTINCT o_c_id FROM orders WHERE o_id = ol_o_id AND o_d_id = ol_d_id AND o_w_id = d_w_id) AS c_id,
           sum(ol_amount) AS sum_amount
    FROM order_line_update
    GROUP BY ol_d_id, ol_o_id
  ) AS inner_sum
  INTO d_id_array, c_id_array, sum_amounts;

  -- Update customer balances
  UPDATE customer
  SET c_balance = COALESCE(c_balance,0) + ids_and_sums.sum_amounts,
      c_delivery_cnt = c_delivery_cnt + 1
  FROM UNNEST(d_id_array, c_id_array, sum_amounts) AS ids_and_sums(d_id, c_id, sum_amounts)
  WHERE customer.c_id = ids_and_sums.c_id
    AND c_d_id = ids_and_sums.d_id
    AND c_w_id = d_w_id;

  RETURN 1;
EXCEPTION
  WHEN serialization_failure OR deadlock_detected OR no_data_found
  THEN ROLLBACK; RETURN -1;
END;
$$ LANGUAGE 'plpgsql';

--= query
CREATE OR REPLACE FUNCTION OSTAT (
  os_w_id INTEGER,
  os_d_id INTEGER,
  os_c_id INTEGER,
  byname INTEGER,
  os_c_last VARCHAR
) RETURNS TABLE(customer_info TEXT, order_info TEXT) AS $$
DECLARE
  namecnt INTEGER;
  os_c_balance NUMERIC;
  os_c_first VARCHAR;
  os_c_middle VARCHAR;
  os_o_id INTEGER;
  os_entdate TIMESTAMP;
  os_o_carrier_id INTEGER;
BEGIN
  IF ( byname = 1 )
  THEN
    SELECT count(c_id) INTO namecnt
    FROM customer
    WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id;

    SELECT c_balance, c_first, c_middle, c_id
    INTO os_c_balance, os_c_first, os_c_middle, os_c_id
    FROM customer
    WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id
    ORDER BY c_first
    LIMIT 1 OFFSET ((namecnt + 1) / 2);
  ELSE
    SELECT c_balance, c_first, c_middle, c_last
    INTO os_c_balance, os_c_first, os_c_middle, os_c_last
    FROM customer
    WHERE c_id = os_c_id AND c_d_id = os_d_id AND c_w_id = os_w_id;
  END IF;

  SELECT o_id, o_carrier_id, o_entry_d
  INTO os_o_id, os_o_carrier_id, os_entdate
  FROM orders
  WHERE o_d_id = os_d_id AND o_w_id = os_w_id AND o_c_id = os_c_id
  ORDER BY o_id DESC
  LIMIT 1;

  RETURN QUERY SELECT
    CAST(os_c_id || '|' || os_c_first || '|' || os_c_middle || '|' || os_c_balance AS TEXT) as customer_info,
    CAST(os_o_id || '|' || os_o_carrier_id || '|' || os_entdate AS TEXT) as order_info;

EXCEPTION
  WHEN serialization_failure OR deadlock_detected OR no_data_found
  THEN RETURN;
END;
$$ LANGUAGE 'plpgsql';

--= query
CREATE OR REPLACE FUNCTION SLEV (
  st_w_id INTEGER,
  st_d_id INTEGER,
  threshold INTEGER
) RETURNS INTEGER AS $$
DECLARE
  stock_count INTEGER;
BEGIN
  SELECT COUNT(DISTINCT (s_i_id)) INTO stock_count
  FROM order_line, stock, district
  WHERE ol_w_id = st_w_id
    AND ol_d_id = st_d_id
    AND d_w_id = st_w_id
    AND d_id = st_d_id
    AND (ol_o_id < d_next_o_id)
    AND ol_o_id >= (d_next_o_id - 20)
    AND s_w_id = st_w_id
    AND s_i_id = ol_i_id
    AND s_quantity < threshold;

  RETURN stock_count;
EXCEPTION
  WHEN serialization_failure OR deadlock_detected OR no_data_found
  THEN RETURN -1;
END;
$$ LANGUAGE 'plpgsql';

--+ workload
--= query
SELECT NEWORD(:w_id, :max_w_id, :d_id, :c_id, :ol_cnt, 0)
--= query
SELECT PAYMENT(:p_w_id, :p_d_id, :p_c_w_id, :p_c_d_id, :p_c_id, :byname, :h_amount, :c_last)
--= query
SELECT * FROM OSTAT(:os_w_id, :os_d_id, :os_c_id, :byname, :os_c_last)
--= query
SELECT DELIVERY(:d_w_id, :d_o_carrier_id);
--= query
SELECT SLEV(:st_w_id, :st_d_id, :threshold);
