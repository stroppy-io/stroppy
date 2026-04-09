--+ drop_schema
--= set_foreign_keys
SET FOREIGN_KEY_CHECKS = 0
--= drop_neword
DROP PROCEDURE IF EXISTS NEWORD
--= drop_payment
DROP PROCEDURE IF EXISTS PAYMENT
--= drop_delivery
DROP PROCEDURE IF EXISTS DELIVERY
--= drop_ostat
DROP PROCEDURE IF EXISTS OSTAT
--= drop_slev
DROP PROCEDURE IF EXISTS SLEV
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

--+ create_procedures
--= neword
CREATE PROCEDURE NEWORD(
  IN no_w_id INT,
  IN no_max_w_id INT,
  IN no_d_id INT,
  IN no_c_id INT,
  IN no_o_ol_cnt INT,
  IN no_force_rollback BOOLEAN
)
BEGIN
  DECLARE no_c_discount DECIMAL(4,4);
  DECLARE no_c_last VARCHAR(16);
  DECLARE no_c_credit CHAR(2);
  DECLARE no_d_tax DECIMAL(4,4);
  DECLARE no_w_tax DECIMAL(4,4);
  DECLARE no_d_next_o_id INT;
  DECLARE no_o_all_local INT;
  DECLARE v_i_id INT;
  DECLARE v_supply_w_id INT;
  DECLARE v_quantity INT;
  DECLARE v_s_quantity INT;
  DECLARE v_i_price DECIMAL(5,2);
  DECLARE v_i_name VARCHAR(24);
  DECLARE v_i_data VARCHAR(50);
  DECLARE v_s_data VARCHAR(50);
  DECLARE v_dist_info CHAR(24);
  DECLARE v_amount DECIMAL(12,2);
  DECLARE loop_counter INT;
  DECLARE item_not_found INT DEFAULT 0;
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET item_not_found = 1;

  SET no_o_all_local = 1;

  SELECT c_discount, c_last, c_credit
    INTO no_c_discount, no_c_last, no_c_credit
  FROM customer
  WHERE c_w_id = no_w_id AND c_d_id = no_d_id AND c_id = no_c_id;

  SELECT w_tax INTO no_w_tax FROM warehouse WHERE w_id = no_w_id;

  /* T2.1: FOR UPDATE to serialize the read-then-increment under
     READ COMMITTED / REPEATABLE READ — InnoDB takes a record lock on
     the district row, blocking concurrent NEWORD VUs until commit, so
     the subsequent INSERT INTO new_order (no_d_next_o_id, ...) can't
     collide on (w_id, d_id, o_id) PK. Previously observed ~0.1% error
     rate on Duplicate entry 'W-D-O' for key 'new_order.PRIMARY'. */
  SELECT d_next_o_id, d_tax INTO no_d_next_o_id, no_d_tax
  FROM district WHERE d_id = no_d_id AND d_w_id = no_w_id
  FOR UPDATE;

  UPDATE district SET d_next_o_id = d_next_o_id + 1
  WHERE d_id = no_d_id AND d_w_id = no_w_id;

  INSERT INTO new_order (no_o_id, no_d_id, no_w_id)
  VALUES (no_d_next_o_id, no_d_id, no_w_id);

  SET loop_counter = 1;
  WHILE loop_counter <= no_o_ol_cnt DO
    SET v_i_id = 1 + FLOOR(RAND() * 100000);
    /* TPC-C 2.4.1.4 forced-rollback sentinel: override the LAST line's i_id
       to a value guaranteed to miss the item table, forcing the SIGNAL path
       below. Driven by client-side 1% roll. */
    IF no_force_rollback AND loop_counter = no_o_ol_cnt THEN
      SET v_i_id = 100001;
    END IF;
    /* TPC-C 2.4.1.5: ~1% of order lines pick a remote supply warehouse
       (uniform over {1..no_max_w_id} \ {no_w_id}) when multiple warehouses exist. */
    IF no_max_w_id > 1 AND FLOOR(RAND() * 100) = 0 THEN
      SET v_supply_w_id = 1 + FLOOR(RAND() * (no_max_w_id - 1));
      IF v_supply_w_id >= no_w_id THEN
        SET v_supply_w_id = v_supply_w_id + 1;
      END IF;
      SET no_o_all_local = 0;
    ELSE
      SET v_supply_w_id = no_w_id;
    END IF;
    SET v_quantity = 1 + FLOOR(RAND() * 10);
    SET item_not_found = 0;

    SELECT i_price, i_name, i_data INTO v_i_price, v_i_name, v_i_data
    FROM item WHERE i_id = v_i_id;

    /* TPC-C 2.4.2.3: on the sentinel-forced last line, raise so the client
       gets a recognizable error and can count the rollback. Non-forced misses
       remain silent (legacy behavior). */
    IF item_not_found = 1 AND no_force_rollback AND loop_counter = no_o_ol_cnt THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'tpcc_rollback:item_not_found';
    END IF;

    IF item_not_found = 0 THEN
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
        SET v_s_quantity = v_s_quantity - v_quantity;
      ELSE
        SET v_s_quantity = v_s_quantity - v_quantity + 91;
      END IF;

      UPDATE stock
        SET s_quantity = v_s_quantity,
            s_ytd = s_ytd + v_quantity,
            s_order_cnt = s_order_cnt + 1,
            s_remote_cnt = s_remote_cnt + CASE WHEN v_supply_w_id <> no_w_id THEN 1 ELSE 0 END
      WHERE s_i_id = v_i_id AND s_w_id = v_supply_w_id;

      SET v_amount = v_quantity * v_i_price;

      INSERT INTO order_line
        (ol_o_id, ol_d_id, ol_w_id, ol_number, ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_dist_info)
      VALUES
        (no_d_next_o_id, no_d_id, no_w_id, loop_counter, v_i_id, v_supply_w_id, v_quantity, v_amount, v_dist_info);
    END IF;

    SET loop_counter = loop_counter + 1;
  END WHILE;

  /* Insert orders after the loop so o_all_local reflects the actual remote flag. */
  INSERT INTO orders (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)
  VALUES (no_d_next_o_id, no_d_id, no_w_id, no_c_id, NOW(), no_o_ol_cnt, no_o_all_local);
END
--= payment
CREATE PROCEDURE PAYMENT(
  IN p_w_id INT,
  IN p_d_id INT,
  IN p_c_w_id INT,
  IN p_c_d_id INT,
  IN p_c_id_in INT,
  IN byname INT,
  IN p_h_amount DECIMAL(6,2),
  IN p_c_last_in VARCHAR(16),
  IN p_h_id BIGINT
)
BEGIN
  DECLARE p_c_balance DECIMAL(12,2);
  DECLARE p_c_credit CHAR(2);
  DECLARE p_c_last VARCHAR(16);
  DECLARE p_c_id INT;
  DECLARE p_w_name VARCHAR(10);
  DECLARE p_d_name VARCHAR(10);
  DECLARE name_count INT;
  DECLARE v_offset INT;
  DECLARE h_data_val VARCHAR(30);

  SET p_c_id = p_c_id_in;
  SET p_c_last = p_c_last_in;

  UPDATE warehouse SET w_ytd = w_ytd + p_h_amount WHERE w_id = p_w_id;
  SELECT w_name INTO p_w_name FROM warehouse WHERE w_id = p_w_id;

  UPDATE district SET d_ytd = d_ytd + p_h_amount
  WHERE d_w_id = p_w_id AND d_id = p_d_id;
  SELECT d_name INTO p_d_name FROM district
  WHERE d_w_id = p_w_id AND d_id = p_d_id;

  IF byname = 1 THEN
    SELECT COUNT(c_last) INTO name_count
    FROM customer
    WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id;

    IF name_count > 0 THEN
      /* TPC-C 2.5.2.2: pick row ceil(n/2). For 0-indexed OFFSET this is (n-1)/2.
         MySQL LIMIT/OFFSET only accepts literals or local variables. */
      SET v_offset = (name_count - 1) DIV 2;
      SELECT c_id, c_balance, c_credit
      INTO p_c_id, p_c_balance, p_c_credit
      FROM customer
      WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id
      ORDER BY c_first
      LIMIT 1 OFFSET v_offset;
    END IF;
  ELSE
    SELECT c_balance, c_credit
    INTO p_c_balance, p_c_credit
    FROM customer
    WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;
  END IF;

  /* TPC-C 2.5.2.2: h_data = W_NAME || '    ' || D_NAME (4 spaces). */
  SET h_data_val = CONCAT(COALESCE(p_w_name, ''), '    ', COALESCE(p_d_name, ''));

  /* TPC-C 2.5.2.2: BC-credit customers get a 500-char c_data log
     prepended with the current Payment's identifying tuple; GC
     customers keep their c_data untouched. MySQL dialect: use
     CONCAT (no '||' string operator by default) and CAST(... AS CHAR)
     for numeric-to-text. DECIMAL(6,2)→CHAR preserves the two-decimal
     form natively, unlike FORMAT() which adds locale thousand
     separators. */
  UPDATE customer
  SET c_balance = c_balance - p_h_amount,
      c_ytd_payment = c_ytd_payment + p_h_amount,
      c_payment_cnt = c_payment_cnt + 1,
      c_data = CASE
        WHEN c_credit = 'BC' THEN SUBSTR(
          CONCAT(
            CAST(c_id AS CHAR), ' ', CAST(c_d_id AS CHAR), ' ', CAST(c_w_id AS CHAR),
            ' ', CAST(p_d_id AS CHAR), ' ', CAST(p_w_id AS CHAR),
            ' ', CAST(p_h_amount AS CHAR), '|', COALESCE(c_data, '')
          ),
          1, 500
        )
        ELSE c_data
      END
  WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;

  INSERT INTO history (h_id, h_c_d_id, h_c_w_id, h_c_id, h_d_id, h_w_id, h_date, h_amount, h_data)
  VALUES (p_h_id, p_c_d_id, p_c_w_id, p_c_id, p_d_id, p_w_id, NOW(), p_h_amount, h_data_val);
END
--= delivery
CREATE PROCEDURE DELIVERY(
  IN d_w_id INT,
  IN d_o_carrier_id INT
)
BEGIN
  DECLARE v_d_id INT;
  DECLARE v_no_o_id INT;
  DECLARE v_c_id INT;
  DECLARE v_ol_total DECIMAL(12,2);

  SET v_d_id = 1;
  WHILE v_d_id <= 10 DO
    SET v_no_o_id = NULL;
    SELECT MIN(no_o_id) INTO v_no_o_id
    FROM new_order
    WHERE no_d_id = v_d_id AND no_w_id = d_w_id;

    IF v_no_o_id IS NOT NULL THEN
      DELETE FROM new_order
      WHERE no_o_id = v_no_o_id AND no_d_id = v_d_id AND no_w_id = d_w_id;

      SELECT o_c_id INTO v_c_id
      FROM orders
      WHERE o_id = v_no_o_id AND o_d_id = v_d_id AND o_w_id = d_w_id;

      UPDATE orders SET o_carrier_id = d_o_carrier_id
      WHERE o_id = v_no_o_id AND o_d_id = v_d_id AND o_w_id = d_w_id;

      UPDATE order_line SET ol_delivery_d = NOW()
      WHERE ol_o_id = v_no_o_id AND ol_d_id = v_d_id AND ol_w_id = d_w_id;

      SELECT COALESCE(SUM(ol_amount), 0) INTO v_ol_total
      FROM order_line
      WHERE ol_o_id = v_no_o_id AND ol_d_id = v_d_id AND ol_w_id = d_w_id;

      UPDATE customer
      SET c_balance = c_balance + v_ol_total,
          c_delivery_cnt = c_delivery_cnt + 1
      WHERE c_id = v_c_id AND c_d_id = v_d_id AND c_w_id = d_w_id;
    END IF;

    SET v_d_id = v_d_id + 1;
  END WHILE;
END
--= ostat
CREATE PROCEDURE OSTAT(
  IN os_w_id INT,
  IN os_d_id INT,
  IN os_c_id INT,
  IN byname INT,
  IN os_c_last VARCHAR(16)
)
BEGIN
  DECLARE namecnt INT;
  DECLARE v_offset INT;
  DECLARE v_c_id INT;
  DECLARE v_c_balance DECIMAL(12,2);
  DECLARE v_c_first VARCHAR(16);
  DECLARE v_c_middle CHAR(2);
  DECLARE v_o_id INT;
  DECLARE v_entdate DATETIME;
  DECLARE v_o_carrier_id INT;

  SET v_c_id = os_c_id;

  IF byname = 1 THEN
    SELECT COUNT(c_id) INTO namecnt
    FROM customer
    WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id;

    IF namecnt > 0 THEN
      /* TPC-C 2.6.2.2: pick row ceil(n/2). For 0-indexed OFFSET this is (n-1)/2.
         MySQL LIMIT/OFFSET only accepts literals or local variables. */
      SET v_offset = (namecnt - 1) DIV 2;
      SELECT c_balance, c_first, c_middle, c_id
      INTO v_c_balance, v_c_first, v_c_middle, v_c_id
      FROM customer
      WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id
      ORDER BY c_first
      LIMIT 1 OFFSET v_offset;
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
END
--= slev
CREATE PROCEDURE SLEV(
  IN st_w_id INT,
  IN st_d_id INT,
  IN threshold INT
)
BEGIN
  DECLARE v_next_o_id INT;
  DECLARE stock_count INT;

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
END

--+ workload_procs
--= new_order
CALL NEWORD(:w_id, :max_w_id, :d_id, :c_id, :ol_cnt, :force_rollback)
--= payment
CALL PAYMENT(:p_w_id, :p_d_id, :p_c_w_id, :p_c_d_id, :p_c_id, :byname, :h_amount, :c_last, :p_h_id)
--= order_status
CALL OSTAT(:os_w_id, :os_d_id, :os_c_id, :byname, :os_c_last)
--= delivery
CALL DELIVERY(:d_w_id, :d_o_carrier_id)
--= stock_level
CALL SLEV(:st_w_id, :st_d_id, :threshold)

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
-- every dialect so the TypeScript has no driver-specific branches.
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
