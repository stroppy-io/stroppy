--+ drop_schema
--= set options
SET FOREIGN_KEY_CHECKS = 0
--= drop neword
DROP PROCEDURE IF EXISTS NEWORD
--= drop payment
DROP PROCEDURE IF EXISTS PAYMENT
--= drop delivery
DROP PROCEDURE IF EXISTS DELIVERY
--= drop ostat
DROP PROCEDURE IF EXISTS OSTAT
--= drop slev
DROP PROCEDURE IF EXISTS SLEV
--= drop tables
DROP TABLE IF EXISTS order_line, new_order, orders, history, stock, customer, district, warehouse, item

--+ create_schema
--= set options
SET FOREIGN_KEY_CHECKS = 0
--= warehouse
CREATE TABLE warehouse (
  w_id       INTEGER      NOT NULL,
  w_name     VARCHAR(10),
  w_street_1 VARCHAR(20),
  w_street_2 VARCHAR(20),
  w_city     VARCHAR(20),
  w_state    CHAR(2),
  w_zip      CHAR(9),
  w_tax      DECIMAL(4,4),
  w_ytd      DECIMAL(12,2),
  PRIMARY KEY (w_id)
) ENGINE = InnoDB
--= district
CREATE TABLE district (
  d_id         INTEGER  NOT NULL,
  d_w_id       INTEGER  NOT NULL,
  d_name       VARCHAR(10),
  d_street_1   VARCHAR(20),
  d_street_2   VARCHAR(20),
  d_city       VARCHAR(20),
  d_state      CHAR(2),
  d_zip        CHAR(9),
  d_tax        DECIMAL(4,4),
  d_ytd        DECIMAL(12,2),
  d_next_o_id  INTEGER,
  PRIMARY KEY (d_w_id, d_id)
) ENGINE = InnoDB
--= customer
CREATE TABLE customer (
  c_id           INTEGER  NOT NULL,
  c_d_id         INTEGER  NOT NULL,
  c_w_id         INTEGER  NOT NULL,
  c_first        VARCHAR(16),
  c_middle       CHAR(2),
  c_last         VARCHAR(16),
  c_street_1     VARCHAR(20),
  c_street_2     VARCHAR(20),
  c_city         VARCHAR(20),
  c_state        CHAR(2),
  c_zip          CHAR(9),
  c_phone        CHAR(16),
  c_since        DATETIME,
  c_credit       CHAR(2),
  c_credit_lim   DECIMAL(12,2),
  c_discount     DECIMAL(4,4),
  c_balance      DECIMAL(12,2),
  c_ytd_payment  DECIMAL(12,2),
  c_payment_cnt  INTEGER,
  c_delivery_cnt INTEGER,
  c_data         VARCHAR(500),
  PRIMARY KEY (c_w_id, c_d_id, c_id),
  KEY c_w_id (c_w_id, c_d_id, c_last, c_first)
) ENGINE = InnoDB
--= history
CREATE TABLE history (
  h_c_id   INTEGER,
  h_c_d_id INTEGER,
  h_c_w_id INTEGER,
  h_d_id   INTEGER,
  h_w_id   INTEGER,
  h_date   DATETIME,
  h_amount DECIMAL(6,2),
  h_data   VARCHAR(24)
) ENGINE = InnoDB
--= new_order
CREATE TABLE new_order (
  no_o_id INTEGER  NOT NULL,
  no_d_id INTEGER  NOT NULL,
  no_w_id INTEGER  NOT NULL,
  PRIMARY KEY (no_w_id, no_d_id, no_o_id)
) ENGINE = InnoDB
--= orders
CREATE TABLE orders (
  o_id         INTEGER  NOT NULL,
  o_d_id       INTEGER  NOT NULL,
  o_w_id       INTEGER  NOT NULL,
  o_c_id       INTEGER,
  o_entry_d    DATETIME,
  o_carrier_id INTEGER,
  o_ol_cnt     INTEGER,
  o_all_local  INTEGER,
  PRIMARY KEY (o_w_id, o_d_id, o_id),
  KEY o_w_id (o_w_id, o_d_id, o_c_id, o_id)
) ENGINE = InnoDB
--= order_line
CREATE TABLE order_line (
  ol_o_id        INTEGER  NOT NULL,
  ol_d_id        INTEGER  NOT NULL,
  ol_w_id        INTEGER  NOT NULL,
  ol_number      INTEGER  NOT NULL,
  ol_i_id        INTEGER,
  ol_supply_w_id INTEGER,
  ol_delivery_d  DATETIME,
  ol_quantity    INTEGER,
  ol_amount      DECIMAL(6,2),
  ol_dist_info   CHAR(24),
  PRIMARY KEY (ol_w_id, ol_d_id, ol_o_id, ol_number)
) ENGINE = InnoDB
--= item
CREATE TABLE item (
  i_id    INTEGER  NOT NULL,
  i_im_id INTEGER,
  i_name  VARCHAR(24),
  i_price DECIMAL(5,2),
  i_data  VARCHAR(50),
  PRIMARY KEY (i_id)
) ENGINE = InnoDB
--= stock
CREATE TABLE stock (
  s_i_id       INTEGER  NOT NULL,
  s_w_id       INTEGER  NOT NULL,
  s_quantity   INTEGER,
  s_dist_01    CHAR(24),
  s_dist_02    CHAR(24),
  s_dist_03    CHAR(24),
  s_dist_04    CHAR(24),
  s_dist_05    CHAR(24),
  s_dist_06    CHAR(24),
  s_dist_07    CHAR(24),
  s_dist_08    CHAR(24),
  s_dist_09    CHAR(24),
  s_dist_10    CHAR(24),
  s_ytd        INTEGER,
  s_order_cnt  INTEGER,
  s_remote_cnt INTEGER,
  s_data       VARCHAR(50),
  PRIMARY KEY (s_w_id, s_i_id)
) ENGINE = InnoDB

--+ create_procedures
--= neword
CREATE PROCEDURE NEWORD (
  no_w_id       INTEGER,
  no_max_w_id   INTEGER,
  no_d_id       INTEGER,
  no_c_id       INTEGER,
  no_o_ol_cnt   INTEGER,
  OUT no_c_discount   DECIMAL(4,4),
  OUT no_c_last       VARCHAR(16),
  OUT no_c_credit     VARCHAR(2),
  OUT no_d_tax        DECIMAL(4,4),
  OUT no_w_tax        DECIMAL(4,4),
  OUT no_d_next_o_id  INTEGER,
  IN  ts              DATETIME
)
BEGIN
  DECLARE no_ol_supply_w_id INTEGER;
  DECLARE no_ol_i_id        INTEGER;
  DECLARE no_ol_quantity    INTEGER;
  DECLARE no_o_all_local    INTEGER;
  DECLARE no_i_name         VARCHAR(24);
  DECLARE no_i_price        DECIMAL(5,2);
  DECLARE no_i_data         VARCHAR(50);
  DECLARE no_s_quantity     INTEGER;
  DECLARE no_ol_amount      DECIMAL(6,2);
  DECLARE no_s_dist_01      CHAR(24);
  DECLARE no_s_dist_02      CHAR(24);
  DECLARE no_s_dist_03      CHAR(24);
  DECLARE no_s_dist_04      CHAR(24);
  DECLARE no_s_dist_05      CHAR(24);
  DECLARE no_s_dist_06      CHAR(24);
  DECLARE no_s_dist_07      CHAR(24);
  DECLARE no_s_dist_08      CHAR(24);
  DECLARE no_s_dist_09      CHAR(24);
  DECLARE no_s_dist_10      CHAR(24);
  DECLARE no_ol_dist_info   CHAR(24);
  DECLARE no_s_data         VARCHAR(50);
  DECLARE rbk               INTEGER;
  DECLARE loop_counter      INTEGER;
  DECLARE `Constraint Violation` CONDITION FOR SQLSTATE '23000';
  DECLARE EXIT HANDLER FOR `Constraint Violation` ROLLBACK;
  DECLARE EXIT HANDLER FOR NOT FOUND ROLLBACK;

  SET no_o_all_local = 1;
  SELECT c_discount, c_last, c_credit, w_tax
  INTO no_c_discount, no_c_last, no_c_credit, no_w_tax
  FROM customer, warehouse
  WHERE warehouse.w_id = no_w_id AND customer.c_w_id = no_w_id
    AND customer.c_d_id = no_d_id AND customer.c_id = no_c_id;

  START TRANSACTION;
  SELECT d_next_o_id, d_tax INTO no_d_next_o_id, no_d_tax
  FROM district WHERE d_id = no_d_id AND d_w_id = no_w_id FOR UPDATE;
  UPDATE district SET d_next_o_id = d_next_o_id + 1
  WHERE d_id = no_d_id AND d_w_id = no_w_id;

  SET rbk = FLOOR(1 + (RAND() * 99));
  SET loop_counter = 1;
  WHILE loop_counter <= no_o_ol_cnt DO
    IF ((loop_counter = no_o_ol_cnt) AND (rbk = 1)) THEN
      SET no_ol_i_id = 100001;
    ELSE
      SET no_ol_i_id = FLOOR(1 + (RAND() * 100000));
    END IF;
    IF (FLOOR(1 + (RAND() * 100)) > 1) THEN
      SET no_ol_supply_w_id = no_w_id;
    ELSE
      SET no_ol_supply_w_id = no_w_id;
      SET no_o_all_local = 0;
      WHILE ((no_ol_supply_w_id = no_w_id) AND (no_max_w_id != 1)) DO
        SET no_ol_supply_w_id = FLOOR(1 + (RAND() * no_max_w_id));
      END WHILE;
    END IF;
    SET no_ol_quantity = FLOOR(1 + (RAND() * 10));
    SELECT i_price, i_name, i_data INTO no_i_price, no_i_name, no_i_data
    FROM item WHERE i_id = no_ol_i_id;
    SELECT s_quantity, s_data,
           s_dist_01, s_dist_02, s_dist_03, s_dist_04, s_dist_05,
           s_dist_06, s_dist_07, s_dist_08, s_dist_09, s_dist_10
    INTO no_s_quantity, no_s_data,
         no_s_dist_01, no_s_dist_02, no_s_dist_03, no_s_dist_04, no_s_dist_05,
         no_s_dist_06, no_s_dist_07, no_s_dist_08, no_s_dist_09, no_s_dist_10
    FROM stock WHERE s_i_id = no_ol_i_id AND s_w_id = no_ol_supply_w_id;
    IF (no_s_quantity > no_ol_quantity) THEN
      SET no_s_quantity = no_s_quantity - no_ol_quantity;
    ELSE
      SET no_s_quantity = no_s_quantity - no_ol_quantity + 91;
    END IF;
    UPDATE stock SET s_quantity = no_s_quantity
    WHERE s_i_id = no_ol_i_id AND s_w_id = no_ol_supply_w_id;
    SET no_ol_amount = no_ol_quantity * no_i_price * (1 + no_w_tax + no_d_tax) * (1 - no_c_discount);
    CASE no_d_id
      WHEN 1  THEN SET no_ol_dist_info = no_s_dist_01;
      WHEN 2  THEN SET no_ol_dist_info = no_s_dist_02;
      WHEN 3  THEN SET no_ol_dist_info = no_s_dist_03;
      WHEN 4  THEN SET no_ol_dist_info = no_s_dist_04;
      WHEN 5  THEN SET no_ol_dist_info = no_s_dist_05;
      WHEN 6  THEN SET no_ol_dist_info = no_s_dist_06;
      WHEN 7  THEN SET no_ol_dist_info = no_s_dist_07;
      WHEN 8  THEN SET no_ol_dist_info = no_s_dist_08;
      WHEN 9  THEN SET no_ol_dist_info = no_s_dist_09;
      WHEN 10 THEN SET no_ol_dist_info = no_s_dist_10;
    END CASE;
    INSERT INTO order_line (ol_o_id, ol_d_id, ol_w_id, ol_number, ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_dist_info)
    VALUES (no_d_next_o_id, no_d_id, no_w_id, loop_counter, no_ol_i_id, no_ol_supply_w_id, no_ol_quantity, no_ol_amount, no_ol_dist_info);
    SET loop_counter = loop_counter + 1;
  END WHILE;
  INSERT INTO orders (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)
  VALUES (no_d_next_o_id, no_d_id, no_w_id, no_c_id, ts, no_o_ol_cnt, no_o_all_local);
  INSERT INTO new_order (no_o_id, no_d_id, no_w_id)
  VALUES (no_d_next_o_id, no_d_id, no_w_id);
  COMMIT;
END

--= payment
CREATE PROCEDURE PAYMENT (
  p_w_id      INTEGER,
  p_d_id      INTEGER,
  p_c_w_id    INTEGER,
  p_c_d_id    INTEGER,
  p_c_id_in   INTEGER,
  byname      INTEGER,
  p_h_amount  DECIMAL(6,2),
  p_c_last_in VARCHAR(16),
  IN ts       DATETIME
)
BEGIN
  DECLARE p_c_id       INTEGER;
  DECLARE p_c_last     VARCHAR(16);
  DECLARE namecnt      INTEGER;
  DECLARE p_w_name     VARCHAR(11);
  DECLARE p_d_name     VARCHAR(11);
  DECLARE h_data       VARCHAR(30);
  DECLARE p_c_credit   CHAR(2);
  DECLARE p_c_balance  DECIMAL(12,2);
  DECLARE p_c_data     VARCHAR(500);
  DECLARE p_c_new_data VARCHAR(500);
  DECLARE loop_counter INTEGER;
  DECLARE done         INT DEFAULT 0;
  DECLARE `Constraint Violation` CONDITION FOR SQLSTATE '23000';
  DECLARE c_byname CURSOR FOR
    SELECT c_id FROM customer
    WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id
    ORDER BY c_first;
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = 1;
  DECLARE EXIT HANDLER FOR `Constraint Violation` ROLLBACK;

  SET p_c_id   = p_c_id_in;
  SET p_c_last = p_c_last_in;

  START TRANSACTION;
  UPDATE warehouse SET w_ytd = w_ytd + p_h_amount WHERE w_id = p_w_id;
  SELECT w_name INTO p_w_name FROM warehouse WHERE w_id = p_w_id;
  UPDATE district SET d_ytd = d_ytd + p_h_amount WHERE d_w_id = p_w_id AND d_id = p_d_id;
  SELECT d_name INTO p_d_name FROM district WHERE d_w_id = p_w_id AND d_id = p_d_id;

  IF (byname = 1) THEN
    SELECT COUNT(c_id) INTO namecnt FROM customer
    WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id;
    IF (MOD(namecnt, 2) = 1) THEN
      SET namecnt = namecnt + 1;
    END IF;
    OPEN c_byname;
    SET loop_counter = 0;
    WHILE loop_counter <= (namecnt / 2) DO
      FETCH c_byname INTO p_c_id;
      SET loop_counter = loop_counter + 1;
    END WHILE;
    CLOSE c_byname;
  END IF;

  SELECT c_balance, c_credit INTO p_c_balance, p_c_credit
  FROM customer WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;

  SET p_c_balance = p_c_balance + p_h_amount;
  IF p_c_credit = 'BC' THEN
    SELECT c_data INTO p_c_data
    FROM customer WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;
    SET h_data = CONCAT(p_w_name, ' ', p_d_name);
    SET p_c_new_data = CONCAT(
      CAST(p_c_id AS CHAR), ' ', CAST(p_c_d_id AS CHAR), ' ', CAST(p_c_w_id AS CHAR), ' ',
      CAST(p_d_id AS CHAR), ' ', CAST(p_w_id AS CHAR), ' ',
      CAST(FORMAT(p_h_amount, 2) AS CHAR), CAST(ts AS CHAR), h_data);
    SET p_c_new_data = SUBSTR(CONCAT(p_c_new_data, p_c_data), 1, 500 - LENGTH(p_c_new_data));
    UPDATE customer
    SET c_balance = p_c_balance, c_data = p_c_new_data,
        c_ytd_payment = c_ytd_payment + p_h_amount, c_payment_cnt = c_payment_cnt + 1
    WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;
  ELSE
    UPDATE customer
    SET c_balance = p_c_balance,
        c_ytd_payment = c_ytd_payment + p_h_amount, c_payment_cnt = c_payment_cnt + 1
    WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;
  END IF;

  SET h_data = CONCAT(p_w_name, ' ', p_d_name);
  INSERT INTO history (h_c_d_id, h_c_w_id, h_c_id, h_d_id, h_w_id, h_date, h_amount, h_data)
  VALUES (p_c_d_id, p_c_w_id, p_c_id, p_d_id, p_w_id, ts, p_h_amount, h_data);
  COMMIT;
END

--= delivery
CREATE PROCEDURE DELIVERY (
  d_w_id         INTEGER,
  d_o_carrier_id INTEGER,
  IN ts          DATETIME
)
BEGIN
  DECLARE d_no_o_id    INTEGER;
  DECLARE d_d_id       INTEGER;
  DECLARE d_c_id       INTEGER;
  DECLARE d_ol_total   DECIMAL(6,2);
  DECLARE loop_counter INTEGER;
  DECLARE `Constraint Violation` CONDITION FOR SQLSTATE '23000';
  DECLARE EXIT HANDLER FOR `Constraint Violation` ROLLBACK;

  SET loop_counter = 1;
  START TRANSACTION;
  WHILE loop_counter <= 10 DO
    SET d_d_id = loop_counter;
    SELECT no_o_id INTO d_no_o_id FROM new_order
    WHERE no_w_id = d_w_id AND no_d_id = d_d_id
    ORDER BY no_o_id LIMIT 1;
    DELETE FROM new_order
    WHERE no_w_id = d_w_id AND no_d_id = d_d_id AND no_o_id = d_no_o_id;
    SELECT o_c_id INTO d_c_id FROM orders
    WHERE o_id = d_no_o_id AND o_d_id = d_d_id AND o_w_id = d_w_id;
    UPDATE orders SET o_carrier_id = d_o_carrier_id
    WHERE o_id = d_no_o_id AND o_d_id = d_d_id AND o_w_id = d_w_id;
    UPDATE order_line SET ol_delivery_d = ts
    WHERE ol_o_id = d_no_o_id AND ol_d_id = d_d_id AND ol_w_id = d_w_id;
    SELECT SUM(ol_amount) INTO d_ol_total FROM order_line
    WHERE ol_o_id = d_no_o_id AND ol_d_id = d_d_id AND ol_w_id = d_w_id;
    UPDATE customer
    SET c_balance = c_balance + d_ol_total, c_delivery_cnt = c_delivery_cnt + 1
    WHERE c_id = d_c_id AND c_d_id = d_d_id AND c_w_id = d_w_id;
    SET loop_counter = loop_counter + 1;
  END WHILE;
  COMMIT;
END

--= ostat
CREATE PROCEDURE OSTAT (
  os_w_id             INTEGER,
  os_d_id             INTEGER,
  os_c_id             INTEGER,
  byname              INTEGER,
  os_c_last           VARCHAR(16),
  OUT os_c_first      VARCHAR(16),
  OUT os_c_middle     CHAR(2),
  OUT os_c_balance    DECIMAL(12,2),
  OUT os_o_id         INTEGER,
  OUT os_entdate      DATETIME,
  OUT os_o_carrier_id INTEGER
)
BEGIN
  DECLARE namecnt      INTEGER;
  DECLARE done         INT DEFAULT 0;
  DECLARE loop_counter INTEGER;
  DECLARE local_c_last VARCHAR(16);
  DECLARE local_c_id   INTEGER;
  DECLARE `Constraint Violation` CONDITION FOR SQLSTATE '23000';
  DECLARE c_name CURSOR FOR
    SELECT c_balance, c_first, c_middle, c_id FROM customer
    WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id
    ORDER BY c_first;
  DECLARE EXIT HANDLER FOR `Constraint Violation` ROLLBACK;
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = 1;

  SET local_c_id = os_c_id;
  START TRANSACTION;
  IF (byname = 1) THEN
    SELECT COUNT(c_id) INTO namecnt FROM customer
    WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id;
    IF (MOD(namecnt, 2) = 1) THEN
      SET namecnt = namecnt + 1;
    END IF;
    OPEN c_name;
    SET loop_counter = 0;
    WHILE loop_counter <= (namecnt / 2) DO
      FETCH c_name INTO os_c_balance, os_c_first, os_c_middle, local_c_id;
      SET loop_counter = loop_counter + 1;
    END WHILE;
    CLOSE c_name;
  ELSE
    SELECT c_balance, c_first, c_middle, c_last
    INTO os_c_balance, os_c_first, os_c_middle, local_c_last
    FROM customer WHERE c_id = local_c_id AND c_d_id = os_d_id AND c_w_id = os_w_id;
  END IF;

  SET done = 0;
  SELECT o_id, o_carrier_id, o_entry_d
  INTO os_o_id, os_o_carrier_id, os_entdate
  FROM (
    SELECT o_id, o_carrier_id, o_entry_d FROM orders
    WHERE o_d_id = os_d_id AND o_w_id = os_w_id AND o_c_id = local_c_id
    ORDER BY o_id DESC
  ) AS sb LIMIT 1;
  COMMIT;
END

--= slev
CREATE PROCEDURE SLEV (
  st_w_id   INTEGER,
  st_d_id   INTEGER,
  threshold INTEGER,
  OUT stock_count INTEGER
)
BEGIN
  DECLARE st_o_id INTEGER;
  DECLARE `Constraint Violation` CONDITION FOR SQLSTATE '23000';
  DECLARE EXIT HANDLER FOR `Constraint Violation` ROLLBACK;
  DECLARE EXIT HANDLER FOR NOT FOUND ROLLBACK;

  START TRANSACTION;
  SELECT d_next_o_id INTO st_o_id FROM district
  WHERE d_w_id = st_w_id AND d_id = st_d_id;
  SELECT COUNT(DISTINCT s_i_id) INTO stock_count
  FROM order_line, stock
  WHERE ol_w_id = st_w_id AND ol_d_id = st_d_id
    AND ol_o_id < st_o_id AND ol_o_id >= (st_o_id - 20)
    AND s_w_id = st_w_id AND s_i_id = ol_i_id
    AND s_quantity < threshold;
  COMMIT;
END

--+ workload
--= new_order
CALL NEWORD(:w_id, :max_w_id, :d_id, :c_id, :ol_cnt, @no_c_discount, @no_c_last, @no_c_credit, @no_d_tax, @no_w_tax, @no_d_next_o_id, NOW())
--= payment
CALL PAYMENT(:p_w_id, :p_d_id, :p_c_w_id, :p_c_d_id, :p_c_id, :byname, :h_amount, :c_last, NOW())
--= order_status
CALL OSTAT(:os_w_id, :os_d_id, :os_c_id, :byname, :os_c_last, @os_c_first, @os_c_middle, @os_c_balance, @os_o_id, @os_entdate, @os_o_carrier_id)
--= delivery
CALL DELIVERY(:d_w_id, :d_o_carrier_id, NOW())
--= stock_level
CALL SLEV(:st_w_id, :st_d_id, :threshold, @stock_count)
