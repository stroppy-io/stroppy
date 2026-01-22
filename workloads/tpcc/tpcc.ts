import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;
import {
  NewDriverByConfig,
  NotifyStep,
  Teardown,
  NewGeneratorByRuleBin,
} from "k6/x/stroppy";

import { Options } from "k6/options";
import {
  GlobalConfig,
  Generation_Rule,
  InsertDescriptor,
  Status,
} from "./stroppy.pb.js";

// Sql Driver interface
interface Driver {
  runQuery(sql: string, args: Record<string, any>): void;
  insertValues(insert: Uint8Array, count: number): void;
}
interface Generator {
  next(): any;
}

declare function NewDriverByConfig(configBin: Uint8Array): Driver;
declare function NotifyStep(name: String, status: Number): void;
declare function Teardown(): Error;
declare function NewGeneratorByRuleBin(
  seed: Number,
  rule: Uint8Array,
): Generator;

declare const __ENV: Record<string, string | undefined>;

function NewGeneratorByRule(seed: Number, rule: Generation_Rule): Generator {
  return NewGeneratorByRuleBin(seed, Generation_Rule.toBinary(rule));
}

const DURATION = __ENV.DURATION || "5m";

export const options: Options = {
  setupTimeout: "5m",
  scenarios: {
    new_order: {
      executor: "constant-vus",
      exec: "new_order",
      vus: 44,
      duration: DURATION,
    },
    payments: {
      executor: "constant-vus",
      exec: "payments",
      vus: 43,
      duration: DURATION,
    },
    order_status: {
      executor: "constant-vus",
      exec: "order_status",
      vus: 4,
      duration: DURATION,
    },
    delivery: {
      executor: "constant-vus",
      exec: "delivery",
      vus: 4,
      duration: DURATION,
    },
    stock_level: {
      executor: "constant-vus",
      exec: "stock_level",
      vus: 4,
      duration: DURATION,
    },
  },
};

// TPCC Configuration Constants
const WAREHOUSES = +(__ENV.SCALE_FACTOR || __ENV.WAREHOUSES || 1);
const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT = 3000;
const ITEMS = 100000;

// Derived constants
const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS =
  WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK = WAREHOUSES * ITEMS;

// Initialize driver with GlobalConfig
const driver = NewDriverByConfig(
  GlobalConfig.toBinary(
    GlobalConfig.create({
      driver: {
        url: __ENV.DRIVER_URL || "postgres://postgres:postgres@localhost:5432",
        driverType: 1,
        dbSpecific: {
          fields: [
            {
              type: { oneofKind: "string", string: "error" },
              key: "trace_log_level",
            },
            {
              type: { oneofKind: "string", string: "5m" },
              key: "max_conn_lifetime",
            },
            {
              type: { oneofKind: "string", string: "2m" },
              key: "max_conn_idle_time",
            },
            { type: { oneofKind: "int32", int32: 1 }, key: "max_conns" },
            { type: { oneofKind: "int32", int32: 1 }, key: "min_conns" },
            { type: { oneofKind: "int32", int32: 1 }, key: "min_idle_conns" },
          ],
        },
      },
    }),
  ),
);

export function setup() {
  NotifyStep("create_schema", Status.STATUS_RUNNING);
  const dropStatements = [
    "DROP FUNCTION IF EXISTS SLEV",
    "DROP FUNCTION IF EXISTS OSTAT",
    "DROP FUNCTION IF EXISTS DELIVERY",
    "DROP FUNCTION IF EXISTS PAYMENT",
    "DROP FUNCTION IF EXISTS NEWORD",
    "DROP FUNCTION IF EXISTS DBMS_RANDOM",
    "DROP TABLE IF EXISTS order_line CASCADE",
    "DROP TABLE IF EXISTS new_order CASCADE",
    "DROP TABLE IF EXISTS orders CASCADE",
    "DROP TABLE IF EXISTS history CASCADE",
    "DROP TABLE IF EXISTS stock CASCADE",
    "DROP TABLE IF EXISTS customer CASCADE",
    "DROP TABLE IF EXISTS district CASCADE",
    "DROP TABLE IF EXISTS warehouse CASCADE",
    "DROP TABLE IF EXISTS item CASCADE",
  ];
  dropStatements.forEach((sql) => driver.runQuery(sql, {}));

  const schemaStatements = [
    `CREATE TABLE warehouse (
  w_id INTEGER PRIMARY KEY,
  w_name VARCHAR(10),
  w_street_1 VARCHAR(20),
  w_street_2 VARCHAR(20),
  w_city VARCHAR(20),
  w_state CHAR(2),
  w_zip CHAR(9),
  w_tax DECIMAL(4,4),
  w_ytd DECIMAL(12,2)
)`,
    `CREATE TABLE district (
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
)`,
    `CREATE TABLE customer (
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
)`,
    `CREATE TABLE history (
  h_c_id INTEGER,
  h_c_d_id INTEGER,
  h_c_w_id INTEGER,
  h_d_id INTEGER,
  h_w_id INTEGER,
  h_date TIMESTAMP,
  h_amount DECIMAL(6,2),
  h_data VARCHAR(24)
)`,
    `CREATE TABLE new_order (
  no_o_id INTEGER,
  no_d_id INTEGER,
  no_w_id INTEGER REFERENCES warehouse(w_id),
  PRIMARY KEY (no_w_id, no_d_id, no_o_id)
)`,
    `CREATE TABLE orders (
  o_id INTEGER,
  o_d_id INTEGER,
  o_w_id INTEGER REFERENCES warehouse(w_id),
  o_c_id INTEGER,
  o_entry_d TIMESTAMP,
  o_carrier_id INTEGER,
  o_ol_cnt INTEGER,
  o_all_local INTEGER,
  PRIMARY KEY (o_w_id, o_d_id, o_id)
)`,
    `CREATE TABLE order_line (
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
)`,
    `CREATE TABLE item (
  i_id INTEGER PRIMARY KEY,
  i_im_id INTEGER,
  i_name VARCHAR(24),
  i_price DECIMAL(5,2),
  i_data VARCHAR(50)
)`,
    `CREATE TABLE stock (
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
)`,
    `CREATE OR REPLACE FUNCTION DBMS_RANDOM (INTEGER, INTEGER) RETURNS INTEGER AS $$
DECLARE
  start_int ALIAS FOR $1;
  end_int ALIAS FOR $2;
BEGIN
  RETURN trunc(random() * (end_int-start_int + 1) + start_int);
END;
$$ LANGUAGE 'plpgsql' STRICT;
`,
    `CREATE OR REPLACE FUNCTION NEWORD (
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
    IF ( round(DBMS_RANDOM(1,100)) \u003E 1 )
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
`,
    `CREATE OR REPLACE FUNCTION PAYMENT (
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
    LIMIT 1 OFFSET (name_count \u002F 2);
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
`,
    `CREATE OR REPLACE FUNCTION DELIVERY (
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
`,
    `CREATE OR REPLACE FUNCTION OSTAT (
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
    LIMIT 1 OFFSET ((namecnt + 1) \u002F 2);
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
`,
    `CREATE OR REPLACE FUNCTION SLEV (
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
    AND (ol_o_id \u003C d_next_o_id)
    AND ol_o_id \u003E= (d_next_o_id - 20)
    AND s_w_id = st_w_id
    AND s_i_id = ol_i_id
    AND s_quantity \u003C threshold;

  RETURN stock_count;
EXCEPTION
  WHEN serialization_failure OR deadlock_detected OR no_data_found
  THEN RETURN -1;
END;
$$ LANGUAGE 'plpgsql';
`,
  ];

  schemaStatements.forEach((sql) => driver.runQuery(sql, {}));
  NotifyStep("create_schema", Status.STATUS_COMPLETED);

  NotifyStep("load_data", Status.STATUS_RUNNING);
  // Load data into tables using InsertValues with COPY_FROM method
  console.log("Loading items...");
  driver.insertValues(
    InsertDescriptor.toBinary(
      InsertDescriptor.create({
        name: "load_items",
        tableName: "item",
        method: 1,
        params: [
          {
            name: "i_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: { max: ITEMS, min: 1 },
              },
              unique: true,
            },
          },
          {
            name: "i_im_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: { max: ITEMS, min: 1 },
              },
            },
          },
          {
            name: "i_name",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "14",
                },
              },
            },
          },
          {
            name: "i_price",
            generationRule: {
              kind: {
                oneofKind: "floatRange",
                floatRange: { max: 100, min: 1 },
              },
            },
          },
          {
            name: "i_data",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "50",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "26",
                },
              },
            },
          },
        ],
        groups: [],
      }),
    ),
    ITEMS,
  );

  console.log("Loading warehouses...");
  driver.insertValues(
    InsertDescriptor.toBinary(
      InsertDescriptor.create({
        name: "load_warehouses",
        tableName: "warehouse",
        method: 1,
        params: [
          {
            name: "w_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: { max: WAREHOUSES, min: 1 },
              },
              unique: true,
            },
          },
          {
            name: "w_name",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: { maxLen: "10", minLen: "6" },
              },
            },
          },
          {
            name: "w_street_1",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: { maxLen: "20", minLen: "10" },
              },
            },
          },
          {
            name: "w_street_2",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: { maxLen: "20", minLen: "10" },
              },
            },
          },
          {
            name: "w_city",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: { maxLen: "20", minLen: "10" },
              },
            },
          },
          {
            name: "w_state",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: { maxLen: "2", minLen: "2" },
              },
            },
          },
          {
            name: "w_zip",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "9",
                  alphabet: { ranges: [{ max: 57, min: 48 }] },
                  minLen: "9",
                },
              },
            },
          },
          {
            name: "w_tax",
            generationRule: {
              kind: { oneofKind: "floatRange", floatRange: { max: 0.2 } },
            },
          },
          {
            name: "w_ytd",
            generationRule: {
              kind: { oneofKind: "floatConst", floatConst: 300000 },
            },
          },
        ],
        groups: [],
      }),
    ),
    WAREHOUSES,
  );

  console.log("Loading districts...");
  driver.insertValues(
    InsertDescriptor.toBinary(
      InsertDescriptor.create({
        name: "load_districts",
        tableName: "district",
        method: 1,
        params: [
          {
            name: "d_name",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "10",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                    ],
                  },
                  minLen: "6",
                },
              },
            },
          },
          {
            name: "d_street_1",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "20",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "10",
                },
              },
            },
          },
          {
            name: "d_street_2",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "20",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "10",
                },
              },
            },
          },
          {
            name: "d_city",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "20",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "10",
                },
              },
            },
          },
          {
            name: "d_state",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "2",
                  alphabet: { ranges: [{ max: 90, min: 65 }] },
                  minLen: "2",
                },
              },
            },
          },
          {
            name: "d_zip",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "9",
                  alphabet: { ranges: [{ max: 57, min: 48 }] },
                  minLen: "9",
                },
              },
            },
          },
          {
            name: "d_tax",
            generationRule: {
              kind: { oneofKind: "floatRange", floatRange: { max: 0.2 } },
            },
          },
          {
            name: "d_ytd",
            generationRule: {
              kind: { oneofKind: "floatConst", floatConst: 30000 },
            },
          },
          {
            name: "d_next_o_id",
            generationRule: {
              kind: { oneofKind: "int32Const", int32Const: 3001 },
            },
          },
        ],
        groups: [
          {
            name: "district_pk",
            params: [
              {
                name: "d_w_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: { max: WAREHOUSES, min: 1 },
                  },
                  unique: true,
                },
              },
              {
                name: "d_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: { max: DISTRICTS_PER_WAREHOUSE, min: 1 },
                  },
                  unique: true,
                },
              },
            ],
          },
        ],
      }),
    ),
    TOTAL_DISTRICTS,
  );

  console.log("Loading customers...");
  driver.insertValues(
    InsertDescriptor.toBinary(
      InsertDescriptor.create({
        name: "load_customers",
        tableName: "customer",
        method: 1,
        params: [
          {
            name: "c_first",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "16",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                    ],
                  },
                  minLen: "8",
                },
              },
            },
          },
          {
            name: "c_middle",
            generationRule: {
              kind: { oneofKind: "stringConst", stringConst: "OE" },
            },
          },
          {
            name: "c_last",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: { maxLen: "16", minLen: "6" },
              },
              unique: true,
            },
          },
          {
            name: "c_street_1",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "20",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "10",
                },
              },
            },
          },
          {
            name: "c_street_2",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "20",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "10",
                },
              },
            },
          },
          {
            name: "c_city",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "20",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "10",
                },
              },
            },
          },
          {
            name: "c_state",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "2",
                  alphabet: { ranges: [{ max: 90, min: 65 }] },
                  minLen: "2",
                },
              },
            },
          },
          {
            name: "c_zip",
            generationRule: {
              kind: { oneofKind: "stringConst", stringConst: "123456789" },
            },
          },
          {
            name: "c_phone",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "16",
                  alphabet: { ranges: [{ max: 57, min: 48 }] },
                  minLen: "16",
                },
              },
            },
          },
          {
            name: "c_since",
            generationRule: {
              kind: {
                oneofKind: "datetimeConst",
                datetimeConst: {
                  value: { seconds: "1761545738", nanos: 810290275 },
                },
              },
            },
          },
          {
            name: "c_credit",
            generationRule: {
              kind: { oneofKind: "stringConst", stringConst: "GC" },
            },
          },
          {
            name: "c_credit_lim",
            generationRule: {
              kind: { oneofKind: "floatConst", floatConst: 50000 },
            },
          },
          {
            name: "c_discount",
            generationRule: {
              kind: { oneofKind: "floatRange", floatRange: { max: 0.5 } },
            },
          },
          {
            name: "c_balance",
            generationRule: {
              kind: { oneofKind: "floatConst", floatConst: -10 },
            },
          },
          {
            name: "c_ytd_payment",
            generationRule: {
              kind: { oneofKind: "floatConst", floatConst: 10 },
            },
          },
          {
            name: "c_payment_cnt",
            generationRule: {
              kind: { oneofKind: "int32Const", int32Const: 1 },
            },
          },
          {
            name: "c_delivery_cnt",
            generationRule: {
              kind: { oneofKind: "int32Const", int32Const: 0 },
            },
          },
          {
            name: "c_data",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "500",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "300",
                },
              },
            },
          },
        ],
        groups: [
          {
            name: "customer_pk",
            params: [
              {
                name: "c_d_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: { max: DISTRICTS_PER_WAREHOUSE, min: 1 },
                  },
                  unique: true,
                },
              },
              {
                name: "c_w_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: { max: WAREHOUSES, min: 1 },
                  },
                  unique: true,
                },
              },
              {
                name: "c_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: { max: CUSTOMERS_PER_DISTRICT, min: 1 },
                  },
                  unique: true,
                },
              },
            ],
          },
        ],
      }),
    ),
    TOTAL_CUSTOMERS,
  );

  console.log("Loading stock...");
  driver.insertValues(
    InsertDescriptor.toBinary(
      InsertDescriptor.create({
        name: "load_stock",
        tableName: "stock",
        method: 1,
        params: [
          {
            name: "s_quantity",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: { max: 100, min: 10 },
              },
            },
          },
          {
            name: "s_dist_01",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_dist_02",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_dist_03",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_dist_04",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_dist_05",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_dist_06",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_dist_07",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_dist_08",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_dist_09",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_dist_10",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "24",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                    ],
                  },
                  minLen: "24",
                },
              },
            },
          },
          {
            name: "s_ytd",
            generationRule: {
              kind: { oneofKind: "int32Const", int32Const: 0 },
            },
          },
          {
            name: "s_order_cnt",
            generationRule: {
              kind: { oneofKind: "int32Const", int32Const: 0 },
            },
          },
          {
            name: "s_remote_cnt",
            generationRule: {
              kind: { oneofKind: "int32Const", int32Const: 0 },
            },
          },
          {
            name: "s_data",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "50",
                  alphabet: {
                    ranges: [
                      { max: 90, min: 65 },
                      { max: 122, min: 97 },
                      { max: 57, min: 48 },
                      { max: 33, min: 32 },
                    ],
                  },
                  minLen: "26",
                },
              },
            },
          },
        ],
        groups: [
          {
            name: "stock_pk",
            params: [
              {
                name: "s_i_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: { max: ITEMS, min: 1 },
                  },
                  unique: true,
                },
              },
              {
                name: "s_w_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: { max: WAREHOUSES, min: 1 },
                  },
                  unique: true,
                },
              },
            ],
          },
        ],
      }),
    ),
    TOTAL_STOCK,
  );

  console.log("Data loading completed!");
  NotifyStep("load_data", Status.STATUS_COMPLETED);

  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}

const newOrderWarehouseGen = NewGeneratorByRule(
  0,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { max: WAREHOUSES, min: 1 } },
  }),
);
const newOrderMaxWarehouseGen = NewGeneratorByRule(
  1,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: {
        max: DISTRICTS_PER_WAREHOUSE,
        min: DISTRICTS_PER_WAREHOUSE,
      },
    },
  }),
);
const newOrderDistrictGen = NewGeneratorByRule(
  2,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: { max: DISTRICTS_PER_WAREHOUSE, min: 1 },
    },
  }),
);
const newOrderCustomerGen = NewGeneratorByRule(
  3,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: { max: CUSTOMERS_PER_DISTRICT, min: 1 },
    },
  }),
);
const newOrderOlCntGen = NewGeneratorByRule(
  4,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { max: 15, min: 5 } },
  }),
);
export function new_order() {
  driver.runQuery(
    "SELECT NEWORD(:w_id, :max_w_id, :d_id, :c_id, :ol_cnt, 0)",
    {
      w_id: newOrderWarehouseGen.next(),
      max_w_id: newOrderMaxWarehouseGen.next(),
      d_id: newOrderDistrictGen.next(),
      c_id: newOrderCustomerGen.next(),
      ol_cnt: newOrderOlCntGen.next(),
    },
  );
}

const paymentWarehouseGen = NewGeneratorByRule(
  5,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { max: WAREHOUSES, min: 1 } },
  }),
);
const paymentDistrictGen = NewGeneratorByRule(
  6,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: { max: DISTRICTS_PER_WAREHOUSE, min: 1 },
    },
  }),
);
const paymentCustomerWarehouseGen = NewGeneratorByRule(
  7,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { max: WAREHOUSES, min: 1 } },
  }),
);
const paymentCustomerDistrictGen = NewGeneratorByRule(
  8,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: { max: DISTRICTS_PER_WAREHOUSE, min: 1 },
    },
  }),
);
const paymentCustomerGen = NewGeneratorByRule(
  9,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: { max: CUSTOMERS_PER_DISTRICT, min: 1 },
    },
  }),
);
const paymentAmountGen = NewGeneratorByRule(
  10,
  Generation_Rule.create({
    kind: { oneofKind: "doubleRange", doubleRange: { max: 5000, min: 1 } },
  }),
);
const paymentCustomerLastGen = NewGeneratorByRule(
  11,
  Generation_Rule.create({
    kind: { oneofKind: "stringRange", stringRange: { maxLen: "16", minLen: "6" } },
    unique: true,
  }),
);
export function payments() {
  driver.runQuery(
    "SELECT PAYMENT(:p_w_id, :p_d_id, :p_c_w_id, :p_c_d_id, :p_c_id, :byname, :h_amount, :c_last)",
    {
      p_w_id: paymentWarehouseGen.next(),
      p_d_id: paymentDistrictGen.next(),
      p_c_w_id: paymentCustomerWarehouseGen.next(),
      p_c_d_id: paymentCustomerDistrictGen.next(),
      p_c_id: paymentCustomerGen.next(),
      byname: 0,
      h_amount: paymentAmountGen.next(),
      c_last: paymentCustomerLastGen.next(),
    },
  );
}

const orderStatusWarehouseGen = NewGeneratorByRule(
  12,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { max: WAREHOUSES, min: 1 } },
  }),
);
const orderStatusDistrictGen = NewGeneratorByRule(
  13,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: { max: DISTRICTS_PER_WAREHOUSE, min: 1 },
    },
  }),
);
const orderStatusCustomerGen = NewGeneratorByRule(
  14,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: { max: CUSTOMERS_PER_DISTRICT, min: 1 },
    },
  }),
);
const orderStatusCustomerLastGen = NewGeneratorByRule(
  15,
  Generation_Rule.create({
    kind: { oneofKind: "stringRange", stringRange: { maxLen: "16", minLen: "8" } },
  }),
);
export function order_status() {
  driver.runQuery(
    "SELECT * FROM OSTAT(:os_w_id, :os_d_id, :os_c_id, :byname, :os_c_last)",
    {
      os_w_id: orderStatusWarehouseGen.next(),
      os_d_id: orderStatusDistrictGen.next(),
      os_c_id: orderStatusCustomerGen.next(),
      byname: 0,
      os_c_last: orderStatusCustomerLastGen.next(),
    },
  );
}

const deliveryWarehouseGen = NewGeneratorByRule(
  16,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { max: WAREHOUSES, min: 1 } },
  }),
);
const deliveryCarrierGen = NewGeneratorByRule(
  17,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: { max: DISTRICTS_PER_WAREHOUSE, min: 1 },
    },
  }),
);
export function delivery() {
  driver.runQuery("SELECT DELIVERY(:d_w_id, :d_o_carrier_id)", {
    d_w_id: deliveryWarehouseGen.next(),
    d_o_carrier_id: deliveryCarrierGen.next(),
  });
}

const stockLevelWarehouseGen = NewGeneratorByRule(
  18,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { max: WAREHOUSES, min: 1 } },
  }),
);
const stockLevelDistrictGen = NewGeneratorByRule(
  19,
  Generation_Rule.create({
    kind: {
      oneofKind: "int32Range",
      int32Range: { max: DISTRICTS_PER_WAREHOUSE, min: 1 },
    },
  }),
);
const stockLevelThresholdGen = NewGeneratorByRule(
  20,
  Generation_Rule.create({
    kind: { oneofKind: "int32Range", int32Range: { max: 20, min: 10 } },
  }),
);
export function stock_level() {
  driver.runQuery("SELECT SLEV(:st_w_id, :st_d_id, :threshold)", {
    st_w_id: stockLevelWarehouseGen.next(),
    st_d_id: stockLevelDistrictGen.next(),
    threshold: stockLevelThresholdGen.next(),
  });
}

export function teardown() {
  NotifyStep("workload", Status.STATUS_COMPLETED);
  Teardown();
}
