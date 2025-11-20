import encoding from "k6/x/encoding";

if (
  globalThis.TextEncoder === undefined &&
  globalThis.TextDecoder === undefined
) {
  globalThis.TextEncoder = encoding.TextEncoder;
  globalThis.TextDecoder = encoding.TextDecoder;
}

import stroppy from "k6/x/stroppy";

import { Options } from "k6/options";
import {
  UnitDescriptor,
  DriverTransactionStat,
  DriverConfig,
  WorkloadDescriptor,
  InsertDescriptor,
} from "./stroppy.pb.js";

// TPCC Configuration Constants
const WAREHOUSES = 1;
const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT = 3000;
const ITEMS = 100000;

// Derived constants
const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS =
  WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK = WAREHOUSES * ITEMS;

type BinMsg<_T extends any> = Uint8Array;

interface Driver {
  runUnit(unit: BinMsg<UnitDescriptor>): BinMsg<DriverTransactionStat>;
  insertValues(
    insert: BinMsg<InsertDescriptor>,
    count: number,
  ): BinMsg<DriverTransactionStat>;
}
const driver: Driver = stroppy;

function RunUnit(unit: UnitDescriptor): void {
  driver.runUnit(UnitDescriptor.toBinary(unit));
}

function RunUnitBin(unit: BinMsg<UnitDescriptor>): void {
  driver.runUnit(unit);
}

function RunWorkload(wl: WorkloadDescriptor) {
  wl.units
    .map((wu) => wu.descriptor)
    .filter((d) => d !== undefined)
    .forEach((d) => RunUnit(d));
}

export const options: Options = {
  setupTimeout: "5m",
  scenarios: {
    new_order: {
      executor: "constant-vus",
      exec: "new_order",
      vus: 44,
      duration: "5m",
    },
    payments: {
      executor: "constant-vus",
      exec: "payments",
      vus: 43,
      duration: "5m",
    },
    order_status: {
      executor: "constant-vus",
      exec: "order_status",
      vus: 4,
      duration: "5m",
    },
    delivery: {
      executor: "constant-vus",
      exec: "delivery",
      vus: 4,
      duration: "5m",
    },
    stock_level: {
      executor: "constant-vus",
      exec: "stock_level",
      vus: 4,
      duration: "5m",
    },
  },
};

// Init context: each VU gets its own driver with single connection
stroppy.parseConfig(
  DriverConfig.toBinary(
    DriverConfig.create({
      // url: "postgres://postgres:postgres@localhost:5432",
      url: "postgres://st-t-postgres:st-t-postgres@51.250.36.120:54321/st-t-postgres?sslmode=disable&password=st-t-postgres-pass",
      driverType: 1,
      dbSpecific: {
        fields: [
          {
            type: {
              oneofKind: "string",
              string: "error",
            },
            key: "trace_log_level",
          },
          {
            type: {
              oneofKind: "string",
              string: "5m",
            },
            key: "max_conn_lifetime",
          },
          {
            type: {
              oneofKind: "string",
              string: "2m",
            },
            key: "max_conn_idle_time",
          },
          {
            type: {
              oneofKind: "int32",
              int32: 1,
            },
            key: "max_conns",
          },
          {
            type: {
              oneofKind: "int32",
              int32: 1,
            },
            key: "min_conns",
          },
          {
            type: {
              oneofKind: "int32",
              int32: 1,
            },
            key: "min_idle_conns",
          },
        ],
      },
    }),
  ),
);

export function insert() {
  console.log("Fuck the world");
}

export function setup() {
  const workload = WorkloadDescriptor.create({
    name: "create_schema",
    units: [
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "warehouse",
              tableIndexes: [],
              columns: [
                {
                  name: "w_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "w_name",
                  sqlType: "VARCHAR(10)",
                },
                {
                  name: "w_street_1",
                  sqlType: "VARCHAR(20)",
                },
                {
                  name: "w_street_2",
                  sqlType: "VARCHAR(20)",
                },
                {
                  name: "w_city",
                  sqlType: "VARCHAR(20)",
                },
                {
                  name: "w_state",
                  sqlType: "CHAR(2)",
                },
                {
                  name: "w_zip",
                  sqlType: "CHAR(9)",
                },
                {
                  name: "w_tax",
                  sqlType: "DECIMAL(4,4)",
                },
                {
                  name: "w_ytd",
                  sqlType: "DECIMAL(12,2)",
                },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "district",
              tableIndexes: [],
              columns: [
                {
                  name: "d_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "d_w_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                  constraint: "REFERENCES warehouse(w_id)",
                },
                {
                  name: "d_name",
                  sqlType: "VARCHAR(10)",
                },
                {
                  name: "d_street_1",
                  sqlType: "VARCHAR(20)",
                },
                {
                  name: "d_street_2",
                  sqlType: "VARCHAR(20)",
                },
                {
                  name: "d_city",
                  sqlType: "VARCHAR(20)",
                },
                {
                  name: "d_state",
                  sqlType: "CHAR(2)",
                },
                {
                  name: "d_zip",
                  sqlType: "CHAR(9)",
                },
                {
                  name: "d_tax",
                  sqlType: "DECIMAL(4,4)",
                },
                {
                  name: "d_ytd",
                  sqlType: "DECIMAL(12,2)",
                },
                {
                  name: "d_next_o_id",
                  sqlType: "INTEGER",
                },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "customer",
              tableIndexes: [],
              columns: [
                {
                  name: "c_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "c_d_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "c_w_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                  constraint: "REFERENCES warehouse(w_id)",
                },
                {
                  name: "c_first",
                  sqlType: "VARCHAR(16)",
                },
                {
                  name: "c_middle",
                  sqlType: "CHAR(2)",
                },
                {
                  name: "c_last",
                  sqlType: "VARCHAR(16)",
                },
                {
                  name: "c_street_1",
                  sqlType: "VARCHAR(20)",
                },
                {
                  name: "c_street_2",
                  sqlType: "VARCHAR(20)",
                },
                {
                  name: "c_city",
                  sqlType: "VARCHAR(20)",
                },
                {
                  name: "c_state",
                  sqlType: "CHAR(2)",
                },
                {
                  name: "c_zip",
                  sqlType: "CHAR(9)",
                },
                {
                  name: "c_phone",
                  sqlType: "CHAR(16)",
                },
                {
                  name: "c_since",
                  sqlType: "TIMESTAMP",
                },
                {
                  name: "c_credit",
                  sqlType: "CHAR(2)",
                },
                {
                  name: "c_credit_lim",
                  sqlType: "DECIMAL(12,2)",
                },
                {
                  name: "c_discount",
                  sqlType: "DECIMAL(4,4)",
                },
                {
                  name: "c_balance",
                  sqlType: "DECIMAL(12,2)",
                },
                {
                  name: "c_ytd_payment",
                  sqlType: "DECIMAL(12,2)",
                },
                {
                  name: "c_payment_cnt",
                  sqlType: "INTEGER",
                },
                {
                  name: "c_delivery_cnt",
                  sqlType: "INTEGER",
                },
                {
                  name: "c_data",
                  sqlType: "VARCHAR(500)",
                },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "history",
              tableIndexes: [],
              columns: [
                {
                  name: "h_c_id",
                  sqlType: "INTEGER",
                },
                {
                  name: "h_c_d_id",
                  sqlType: "INTEGER",
                },
                {
                  name: "h_c_w_id",
                  sqlType: "INTEGER",
                },
                {
                  name: "h_d_id",
                  sqlType: "INTEGER",
                },
                {
                  name: "h_w_id",
                  sqlType: "INTEGER",
                },
                {
                  name: "h_date",
                  sqlType: "TIMESTAMP",
                },
                {
                  name: "h_amount",
                  sqlType: "DECIMAL(6,2)",
                },
                {
                  name: "h_data",
                  sqlType: "VARCHAR(24)",
                },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "new_order",
              tableIndexes: [],
              columns: [
                {
                  name: "no_o_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "no_d_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "no_w_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                  constraint: "REFERENCES warehouse(w_id)",
                },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "orders",
              tableIndexes: [],
              columns: [
                {
                  name: "o_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "o_d_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "o_w_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                  constraint: "REFERENCES warehouse(w_id)",
                },
                {
                  name: "o_c_id",
                  sqlType: "INTEGER",
                },
                {
                  name: "o_entry_d",
                  sqlType: "TIMESTAMP",
                },
                {
                  name: "o_carrier_id",
                  sqlType: "INTEGER",
                  nullable: true,
                },
                {
                  name: "o_ol_cnt",
                  sqlType: "INTEGER",
                },
                {
                  name: "o_all_local",
                  sqlType: "INTEGER",
                },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "order_line",
              tableIndexes: [],
              columns: [
                {
                  name: "ol_o_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "ol_d_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "ol_w_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                  constraint: "REFERENCES warehouse(w_id)",
                },
                {
                  name: "ol_number",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "ol_i_id",
                  sqlType: "INTEGER",
                },
                {
                  name: "ol_supply_w_id",
                  sqlType: "INTEGER",
                },
                {
                  name: "ol_delivery_d",
                  sqlType: "TIMESTAMP",
                  nullable: true,
                },
                {
                  name: "ol_quantity",
                  sqlType: "INTEGER",
                },
                {
                  name: "ol_amount",
                  sqlType: "DECIMAL(6,2)",
                },
                {
                  name: "ol_dist_info",
                  sqlType: "CHAR(24)",
                },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "item",
              tableIndexes: [],
              columns: [
                {
                  name: "i_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                },
                {
                  name: "i_im_id",
                  sqlType: "INTEGER",
                },
                {
                  name: "i_name",
                  sqlType: "VARCHAR(24)",
                },
                {
                  name: "i_price",
                  sqlType: "DECIMAL(5,2)",
                },
                {
                  name: "i_data",
                  sqlType: "VARCHAR(50)",
                },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "createTable",
            createTable: {
              name: "stock",
              tableIndexes: [],
              columns: [
                {
                  name: "s_i_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                  constraint: "REFERENCES item(i_id)",
                },
                {
                  name: "s_w_id",
                  sqlType: "INTEGER",
                  primaryKey: true,
                  constraint: "REFERENCES warehouse(w_id)",
                },
                {
                  name: "s_quantity",
                  sqlType: "INTEGER",
                },
                {
                  name: "s_dist_01",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_dist_02",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_dist_03",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_dist_04",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_dist_05",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_dist_06",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_dist_07",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_dist_08",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_dist_09",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_dist_10",
                  sqlType: "CHAR(24)",
                },
                {
                  name: "s_ytd",
                  sqlType: "INTEGER",
                },
                {
                  name: "s_order_cnt",
                  sqlType: "INTEGER",
                },
                {
                  name: "s_remote_cnt",
                  sqlType: "INTEGER",
                },
                {
                  name: "s_data",
                  sqlType: "VARCHAR(50)",
                },
              ],
              dbSpecific: {
                fields: [],
              },
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "create_dbms_random",
              sql: `CREATE OR REPLACE FUNCTION DBMS_RANDOM (INTEGER, INTEGER) RETURNS INTEGER AS $$
DECLARE
  start_int ALIAS FOR $1;
  end_int ALIAS FOR $2;
BEGIN
  RETURN trunc(random() * (end_int-start_int + 1) + start_int);
END;
$$ LANGUAGE 'plpgsql' STRICT;
`,
              params: [],
              groups: [],
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "create_neword_procedure",
              sql: `CREATE OR REPLACE FUNCTION NEWORD (
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
              params: [],
              groups: [],
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "create_payment_procedure",
              sql: `CREATE OR REPLACE FUNCTION PAYMENT (
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
              params: [],
              groups: [],
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "create_delivery_procedure",
              sql: `CREATE OR REPLACE FUNCTION DELIVERY (
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
              params: [],
              groups: [],
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "create_ostat_procedure",
              sql: `CREATE OR REPLACE FUNCTION OSTAT (
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
              params: [],
              groups: [],
            },
          },
        },
      },
      {
        count: "1",
        descriptor: {
          type: {
            oneofKind: "query",
            query: {
              name: "create_slev_procedure",
              sql: `CREATE OR REPLACE FUNCTION SLEV (
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
              params: [],
              groups: [],
            },
          },
        },
      },
    ],
  });

  RunWorkload(workload);

  // Load data into tables using InsertValues with COPY_FROM method
  console.log("Loading items...");
  driver.insertValues(
    InsertDescriptor.toBinary(
      InsertDescriptor.create({
        name: "load_items",
        tableName: "item",
        method: 1, // COPY_FROM
        params: [
          {
            name: "i_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: ITEMS,
                  min: 1,
                },
              },
              unique: true,
            },
          },
          {
            name: "i_im_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: ITEMS,
                  min: 1,
                },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
                floatRange: {
                  max: 100,
                  min: 1,
                },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
        method: 1, // COPY_FROM
        params: [
          {
            name: "w_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: WAREHOUSES,
                  min: 1,
                },
              },
              unique: true,
            },
          },
          {
            name: "w_name",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "10",
                  minLen: "6",
                },
              },
            },
          },
          {
            name: "w_street_1",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "20",
                  minLen: "10",
                },
              },
            },
          },
          {
            name: "w_street_2",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "20",
                  minLen: "10",
                },
              },
            },
          },
          {
            name: "w_city",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "20",
                  minLen: "10",
                },
              },
            },
          },
          {
            name: "w_state",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "2",
                  minLen: "2",
                },
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
                  alphabet: {
                    ranges: [
                      {
                        max: 57,
                        min: 48,
                      },
                    ],
                  },
                  minLen: "9",
                },
              },
            },
          },
          {
            name: "w_tax",
            generationRule: {
              kind: {
                oneofKind: "floatRange",
                floatRange: {
                  max: 0.2,
                },
              },
            },
          },
          {
            name: "w_ytd",
            generationRule: {
              kind: {
                oneofKind: "floatConst",
                floatConst: 300000,
              },
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
        method: 1, // COPY_FROM
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
                  alphabet: {
                    ranges: [
                      {
                        max: 90,
                        min: 65,
                      },
                    ],
                  },
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
                  alphabet: {
                    ranges: [
                      {
                        max: 57,
                        min: 48,
                      },
                    ],
                  },
                  minLen: "9",
                },
              },
            },
          },
          {
            name: "d_tax",
            generationRule: {
              kind: {
                oneofKind: "floatRange",
                floatRange: {
                  max: 0.2,
                },
              },
            },
          },
          {
            name: "d_ytd",
            generationRule: {
              kind: {
                oneofKind: "floatConst",
                floatConst: 30000,
              },
            },
          },
          {
            name: "d_next_o_id",
            generationRule: {
              kind: {
                oneofKind: "int32Const",
                int32Const: 3001,
              },
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
                    int32Range: {
                      max: WAREHOUSES,
                      min: 1,
                    },
                  },
                  unique: true,
                },
              },
              {
                name: "d_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: {
                      max: DISTRICTS_PER_WAREHOUSE,
                      min: 1,
                    },
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
        method: 1, // COPY_FROM
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
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
              kind: {
                oneofKind: "stringConst",
                stringConst: "OE",
              },
            },
          },
          {
            name: "c_last",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "16",
                  minLen: "6",
                },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
                  alphabet: {
                    ranges: [
                      {
                        max: 90,
                        min: 65,
                      },
                    ],
                  },
                  minLen: "2",
                },
              },
            },
          },
          {
            name: "c_zip",
            generationRule: {
              kind: {
                oneofKind: "stringConst",
                stringConst: "123456789",
              },
            },
          },
          {
            name: "c_phone",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "16",
                  alphabet: {
                    ranges: [
                      {
                        max: 57,
                        min: 48,
                      },
                    ],
                  },
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
                  value: {
                    seconds: "1761545738",
                    nanos: 810290275,
                  },
                },
              },
            },
          },
          {
            name: "c_credit",
            generationRule: {
              kind: {
                oneofKind: "stringConst",
                stringConst: "GC",
              },
            },
          },
          {
            name: "c_credit_lim",
            generationRule: {
              kind: {
                oneofKind: "floatConst",
                floatConst: 50000,
              },
            },
          },
          {
            name: "c_discount",
            generationRule: {
              kind: {
                oneofKind: "floatRange",
                floatRange: {
                  max: 0.5,
                },
              },
            },
          },
          {
            name: "c_balance",
            generationRule: {
              kind: {
                oneofKind: "floatConst",
                floatConst: -10,
              },
            },
          },
          {
            name: "c_ytd_payment",
            generationRule: {
              kind: {
                oneofKind: "floatConst",
                floatConst: 10,
              },
            },
          },
          {
            name: "c_payment_cnt",
            generationRule: {
              kind: {
                oneofKind: "int32Const",
                int32Const: 1,
              },
            },
          },
          {
            name: "c_delivery_cnt",
            generationRule: {
              kind: {
                oneofKind: "int32Const",
                int32Const: 0,
              },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
                    int32Range: {
                      max: DISTRICTS_PER_WAREHOUSE,
                      min: 1,
                    },
                  },
                  unique: true,
                },
              },
              {
                name: "c_w_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: {
                      max: WAREHOUSES,
                      min: 1,
                    },
                  },
                  unique: true,
                },
              },
              {
                name: "c_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: {
                      max: CUSTOMERS_PER_DISTRICT,
                      min: 1,
                    },
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
        method: 1, // COPY_FROM
        params: [
          {
            name: "s_quantity",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: 100,
                  min: 10,
                },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
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
              kind: {
                oneofKind: "int32Const",
                int32Const: 0,
              },
            },
          },
          {
            name: "s_order_cnt",
            generationRule: {
              kind: {
                oneofKind: "int32Const",
                int32Const: 0,
              },
            },
          },
          {
            name: "s_remote_cnt",
            generationRule: {
              kind: {
                oneofKind: "int32Const",
                int32Const: 0,
              },
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
                      {
                        max: 90,
                        min: 65,
                      },
                      {
                        max: 122,
                        min: 97,
                      },
                      {
                        max: 57,
                        min: 48,
                      },
                      {
                        max: 33,
                        min: 32,
                      },
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
                    int32Range: {
                      max: ITEMS,
                      min: 1,
                    },
                  },
                  unique: true,
                },
              },
              {
                name: "s_w_id",
                generationRule: {
                  kind: {
                    oneofKind: "int32Range",
                    int32Range: {
                      max: WAREHOUSES,
                      min: 1,
                    },
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
  return;
}

const newOrderDesciptorBin: BinMsg<UnitDescriptor> = UnitDescriptor.toBinary(
  UnitDescriptor.create({
    type: {
      oneofKind: "query",
      query: {
        name: "call_neword",
        sql: "SELECT NEWORD(${w_id}, ${max_w_id}, ${d_id}, ${c_id}, ${ol_cnt}, 0)",
        params: [
          {
            name: "w_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: WAREHOUSES,
                  min: 1,
                },
              },
            },
          },
          {
            name: "max_w_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: DISTRICTS_PER_WAREHOUSE,
                  min: DISTRICTS_PER_WAREHOUSE,
                },
              },
            },
          },
          {
            name: "d_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: DISTRICTS_PER_WAREHOUSE,
                  min: 1,
                },
              },
            },
          },
          {
            name: "c_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: CUSTOMERS_PER_DISTRICT,
                  min: 1,
                },
              },
            },
          },
          {
            name: "ol_cnt",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: 15,
                  min: 5,
                },
              },
            },
          },
        ],
        groups: [],
      },
    },
  }),
);
export function new_order() {
  RunUnitBin(newOrderDesciptorBin);
}

const paymentsDescriptorBin: BinMsg<UnitDescriptor> = UnitDescriptor.toBinary(
  UnitDescriptor.create({
    type: {
      oneofKind: "query",
      query: {
        name: "payment_procedure",
        sql: "SELECT PAYMENT(${p_w_id}, ${p_d_id}, ${p_c_w_id}, ${p_c_d_id}, ${p_c_id}, ${byname}, ${h_amount}, ${c_last})",
        params: [
          {
            name: "p_w_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: WAREHOUSES,
                  min: 1,
                },
              },
            },
          },
          {
            name: "p_d_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: DISTRICTS_PER_WAREHOUSE,
                  min: 1,
                },
              },
            },
          },
          {
            name: "p_c_w_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: WAREHOUSES,
                  min: 1,
                },
              },
            },
          },
          {
            name: "p_c_d_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: DISTRICTS_PER_WAREHOUSE,
                  min: 1,
                },
              },
            },
          },
          {
            name: "p_c_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: CUSTOMERS_PER_DISTRICT,
                  min: 1,
                },
              },
            },
          },
          {
            name: "byname",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: 0,
                  min: 0,
                },
              },
            },
          },
          {
            name: "h_amount",
            generationRule: {
              kind: {
                oneofKind: "doubleRange",
                doubleRange: {
                  max: 5000,
                  min: 1,
                },
              },
            },
          },
          {
            name: "c_last",
            generationRule: {
              kind: {
                oneofKind: "stringRange",
                stringRange: {
                  maxLen: "16",
                  minLen: "6",
                },
              },
              unique: true,
            },
          },
        ],
        groups: [],
      },
    },
  }),
);
export function payments() {
  RunUnitBin(paymentsDescriptorBin);
}

const orderStatusDescriptorBin: BinMsg<UnitDescriptor> =
  UnitDescriptor.toBinary(
    UnitDescriptor.create({
      type: {
        oneofKind: "query",
        query: {
          name: "order_status_procedure",
          sql: "SELECT * FROM OSTAT(${os_w_id}, ${os_d_id}, ${os_c_id}, ${byname}, ${os_c_last})",
          params: [
            {
              name: "os_w_id",
              generationRule: {
                kind: {
                  oneofKind: "int32Range",
                  int32Range: {
                    max: WAREHOUSES,
                    min: 1,
                  },
                },
              },
            },
            {
              name: "os_d_id",
              generationRule: {
                kind: {
                  oneofKind: "int32Range",
                  int32Range: {
                    max: DISTRICTS_PER_WAREHOUSE,
                    min: 1,
                  },
                },
              },
            },
            {
              name: "os_c_id",
              generationRule: {
                kind: {
                  oneofKind: "int32Range",
                  int32Range: {
                    max: CUSTOMERS_PER_DISTRICT,
                    min: 1,
                  },
                },
              },
            },
            {
              name: "byname",
              generationRule: {
                kind: {
                  oneofKind: "int32Range",
                  int32Range: {
                    max: 0,
                    min: 0,
                  },
                },
              },
            },
            {
              name: "os_c_last",
              generationRule: {
                kind: {
                  oneofKind: "stringRange",
                  stringRange: {
                    maxLen: "16",
                    minLen: "8",
                  },
                },
              },
            },
          ],
          groups: [],
        },
      },
    }),
  );
export function order_status() {
  RunUnitBin(orderStatusDescriptorBin);
}

const deliveryDescriptorBin: BinMsg<UnitDescriptor> = UnitDescriptor.toBinary(
  UnitDescriptor.create({
    type: {
      oneofKind: "query",
      query: {
        name: "delivery_procedure",
        sql: "SELECT DELIVERY(${d_w_id}, ${d_o_carrier_id})",
        params: [
          {
            name: "d_w_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: WAREHOUSES,
                  min: 1,
                },
              },
            },
          },
          {
            name: "d_o_carrier_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: DISTRICTS_PER_WAREHOUSE,
                  min: 1,
                },
              },
            },
          },
        ],
        groups: [],
      },
    },
  }),
);
export function delivery() {
  RunUnitBin(deliveryDescriptorBin);
}

const stockLevelDescriptorBin: BinMsg<UnitDescriptor> = UnitDescriptor.toBinary(
  UnitDescriptor.create({
    type: {
      oneofKind: "query",
      query: {
        name: "stock_level_transaction",
        sql: "SELECT SLEV(${st_w_id}, ${st_d_id}, ${threshold})",
        params: [
          {
            name: "st_w_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: WAREHOUSES,
                  min: 1,
                },
              },
            },
          },
          {
            name: "st_d_id",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: DISTRICTS_PER_WAREHOUSE,
                  min: 1,
                },
              },
            },
          },
          {
            name: "threshold",
            generationRule: {
              kind: {
                oneofKind: "int32Range",
                int32Range: {
                  max: 20,
                  min: 10,
                },
              },
            },
          },
        ],
        groups: [],
      },
    },
  }),
);
export function stock_level() {
  RunUnitBin(stockLevelDescriptorBin);
}

/*
const config: ConfigFile = {
  exporters: [
    {
      name: "tpcc-metrics",
      otlpExport: {
        otlpGrpcEndpoint: "localhost:4317",
        otlpEndpointInsecure: false,
        otlpMetricsPrefix: "stroppy_k6_tpcc_",
      },
    },
  ],
  executors: [
    {
      name: "single-execution",
      k6: {
        k6Args: [],
        scenario: {
          executor: {
            oneofKind: "perVuIterations",
            perVuIterations: {
              vus: 1,
              iterations: "-1",
            },
          },
          maxDuration: {
            seconds: "3600",
            nanos: 0,
          },
        },
      },
    },
    {
      name: "data-load-executor",
      k6: {
        k6Args: [],
        scenario: {
          executor: {
            oneofKind: "perVuIterations",
            perVuIterations: {
              vus: 1,
              iterations: "-1",
            },
          },
          maxDuration: {
            seconds: "3600",
            nanos: 0,
          },
        },
      },
    },
    {
      name: "tpcc-benchmark",
      k6: {
        k6Args: [],
        scenario: {
          executor: {
            oneofKind: "constantArrivalRate",
            constantArrivalRate: {
              rate: 500,
              preAllocatedVus: 100,
              maxVus: 2000,
              timeUnit: {
                seconds: "1",
                nanos: 0,
              },
              duration: {
                seconds: "1800",
                nanos: 0,
              },
            },
          },
          maxDuration: {
            seconds: "3600",
            nanos: 0,
          },
        },
      },
    },
    {
      name: "tpcc-benchmark-ramping-rate",
      k6: {
        k6Args: [],
        scenario: {
          executor: {
            oneofKind: "rampingArrivalRate",
            rampingArrivalRate: {
              startRate: 500,
              stages: [
                {
                  target: 500,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 1000,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 1500,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 2000,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 2500,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 3000,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 3500,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 4000,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 4500,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 5000,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 5500,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 6000,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
                {
                  target: 6500,
                  duration: {
                    seconds: "600",
                    nanos: 0,
                  },
                },
              ],
              preAllocatedVus: 100,
              maxVus: 2000,
              timeUnit: {
                seconds: "1",
                nanos: 0,
              },
            },
          },
          maxDuration: {
            seconds: "3600",
            nanos: 0,
          },
        },
      },
    },
  ],
  steps: [
    {
      name: "create_schema",
      workload: "create_schema",
      executor: "single-execution",
      exporter: "tpcc-metrics",
    },
    {
      name: "create_procedures",
      workload: "create_stored_procedures",
      executor: "single-execution",
      exporter: "tpcc-metrics",
    },
    {
      name: "load_data",
      workload: "load_data",
      executor: "data-load-executor",
      exporter: "tpcc-metrics",
    },
    {
      name: "tpcc_workload",
      workload: "tpcc_workload",
      executor: "tpcc-benchmark",
      exporter: "tpcc-metrics",
    },
    {
      name: "cleanup",
      workload: "cleanup",
      executor: "single-execution",
      exporter: "tpcc-metrics",
    },
  ],
  sideCars: [],
  global: {
    version: "v1.0.2",
    runId: "dfb62936-8d79-44f5-88a1-8176f0b398c4",
    seed: "1761545738",
    metadata: {
      approach: "stored_procedures",
      benchmark_type: "tpc_c_with_procedures",
      description: "TPC-C Benchmark with Stored Procedures",
      specification_version: "5.11",
      warehouses: String(WAREHOUSES),
    },
    driver: {
      url: "postgres:\u002F\u002Fpostgres:postgres@localhost:5432\u002Fpostgres?sslmode=disable",
      driverType: 1,
      dbSpecific: {
        fields: [
          {
            type: {
              oneofKind: "string",
              string: "warn",
            },
            key: "trace_log_level",
          },
          {
            type: {
              oneofKind: "string",
              string: "5m",
            },
            key: "max_conn_lifetime",
          },
          {
            type: {
              oneofKind: "string",
              string: "2m",
            },
            key: "max_conn_idle_time",
          },
          {
            type: {
              oneofKind: "int32",
              int32: 300,
            },
            key: "max_conns",
          },
          {
            type: {
              oneofKind: "int32",
              int32: 50,
            },
            key: "min_conns",
          },
          {
            type: {
              oneofKind: "int32",
              int32: 100,
            },
            key: "min_idle_conns",
          },
        ],
      },
    },
    logger: {
      logLevel: 1,
      logMode: 1,
    },
  },
  benchmark: {
    name: "tpcc_postgresql",
    workloads: [
      {
        name: "create_schema",
        units: [
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "createTable",
                createTable: {
                  name: "warehouse",
                  tableIndexes: [],
                  columns: [
                    {
                      name: "w_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "w_name",
                      sqlType: "VARCHAR(10)",
                    },
                    {
                      name: "w_street_1",
                      sqlType: "VARCHAR(20)",
                    },
                    {
                      name: "w_street_2",
                      sqlType: "VARCHAR(20)",
                    },
                    {
                      name: "w_city",
                      sqlType: "VARCHAR(20)",
                    },
                    {
                      name: "w_state",
                      sqlType: "CHAR(2)",
                    },
                    {
                      name: "w_zip",
                      sqlType: "CHAR(9)",
                    },
                    {
                      name: "w_tax",
                      sqlType: "DECIMAL(4,4)",
                    },
                    {
                      name: "w_ytd",
                      sqlType: "DECIMAL(12,2)",
                    },
                  ],
                  dbSpecific: {
                    fields: [],
                  },
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "createTable",
                createTable: {
                  name: "district",
                  tableIndexes: [],
                  columns: [
                    {
                      name: "d_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "d_w_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                      constraint: "REFERENCES warehouse(w_id)",
                    },
                    {
                      name: "d_name",
                      sqlType: "VARCHAR(10)",
                    },
                    {
                      name: "d_street_1",
                      sqlType: "VARCHAR(20)",
                    },
                    {
                      name: "d_street_2",
                      sqlType: "VARCHAR(20)",
                    },
                    {
                      name: "d_city",
                      sqlType: "VARCHAR(20)",
                    },
                    {
                      name: "d_state",
                      sqlType: "CHAR(2)",
                    },
                    {
                      name: "d_zip",
                      sqlType: "CHAR(9)",
                    },
                    {
                      name: "d_tax",
                      sqlType: "DECIMAL(4,4)",
                    },
                    {
                      name: "d_ytd",
                      sqlType: "DECIMAL(12,2)",
                    },
                    {
                      name: "d_next_o_id",
                      sqlType: "INTEGER",
                    },
                  ],
                  dbSpecific: {
                    fields: [],
                  },
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "createTable",
                createTable: {
                  name: "customer",
                  tableIndexes: [],
                  columns: [
                    {
                      name: "c_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "c_d_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "c_w_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                      constraint: "REFERENCES warehouse(w_id)",
                    },
                    {
                      name: "c_first",
                      sqlType: "VARCHAR(16)",
                    },
                    {
                      name: "c_middle",
                      sqlType: "CHAR(2)",
                    },
                    {
                      name: "c_last",
                      sqlType: "VARCHAR(16)",
                    },
                    {
                      name: "c_street_1",
                      sqlType: "VARCHAR(20)",
                    },
                    {
                      name: "c_street_2",
                      sqlType: "VARCHAR(20)",
                    },
                    {
                      name: "c_city",
                      sqlType: "VARCHAR(20)",
                    },
                    {
                      name: "c_state",
                      sqlType: "CHAR(2)",
                    },
                    {
                      name: "c_zip",
                      sqlType: "CHAR(9)",
                    },
                    {
                      name: "c_phone",
                      sqlType: "CHAR(16)",
                    },
                    {
                      name: "c_since",
                      sqlType: "TIMESTAMP",
                    },
                    {
                      name: "c_credit",
                      sqlType: "CHAR(2)",
                    },
                    {
                      name: "c_credit_lim",
                      sqlType: "DECIMAL(12,2)",
                    },
                    {
                      name: "c_discount",
                      sqlType: "DECIMAL(4,4)",
                    },
                    {
                      name: "c_balance",
                      sqlType: "DECIMAL(12,2)",
                    },
                    {
                      name: "c_ytd_payment",
                      sqlType: "DECIMAL(12,2)",
                    },
                    {
                      name: "c_payment_cnt",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "c_delivery_cnt",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "c_data",
                      sqlType: "VARCHAR(500)",
                    },
                  ],
                  dbSpecific: {
                    fields: [],
                  },
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "createTable",
                createTable: {
                  name: "history",
                  tableIndexes: [],
                  columns: [
                    {
                      name: "h_c_id",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "h_c_d_id",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "h_c_w_id",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "h_d_id",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "h_w_id",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "h_date",
                      sqlType: "TIMESTAMP",
                    },
                    {
                      name: "h_amount",
                      sqlType: "DECIMAL(6,2)",
                    },
                    {
                      name: "h_data",
                      sqlType: "VARCHAR(24)",
                    },
                  ],
                  dbSpecific: {
                    fields: [],
                  },
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "createTable",
                createTable: {
                  name: "new_order",
                  tableIndexes: [],
                  columns: [
                    {
                      name: "no_o_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "no_d_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "no_w_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                      constraint: "REFERENCES warehouse(w_id)",
                    },
                  ],
                  dbSpecific: {
                    fields: [],
                  },
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "createTable",
                createTable: {
                  name: "orders",
                  tableIndexes: [],
                  columns: [
                    {
                      name: "o_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "o_d_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "o_w_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                      constraint: "REFERENCES warehouse(w_id)",
                    },
                    {
                      name: "o_c_id",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "o_entry_d",
                      sqlType: "TIMESTAMP",
                    },
                    {
                      name: "o_carrier_id",
                      sqlType: "INTEGER",
                      nullable: true,
                    },
                    {
                      name: "o_ol_cnt",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "o_all_local",
                      sqlType: "INTEGER",
                    },
                  ],
                  dbSpecific: {
                    fields: [],
                  },
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "createTable",
                createTable: {
                  name: "order_line",
                  tableIndexes: [],
                  columns: [
                    {
                      name: "ol_o_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "ol_d_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "ol_w_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                      constraint: "REFERENCES warehouse(w_id)",
                    },
                    {
                      name: "ol_number",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "ol_i_id",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "ol_supply_w_id",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "ol_delivery_d",
                      sqlType: "TIMESTAMP",
                      nullable: true,
                    },
                    {
                      name: "ol_quantity",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "ol_amount",
                      sqlType: "DECIMAL(6,2)",
                    },
                    {
                      name: "ol_dist_info",
                      sqlType: "CHAR(24)",
                    },
                  ],
                  dbSpecific: {
                    fields: [],
                  },
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "createTable",
                createTable: {
                  name: "item",
                  tableIndexes: [],
                  columns: [
                    {
                      name: "i_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                    },
                    {
                      name: "i_im_id",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "i_name",
                      sqlType: "VARCHAR(24)",
                    },
                    {
                      name: "i_price",
                      sqlType: "DECIMAL(5,2)",
                    },
                    {
                      name: "i_data",
                      sqlType: "VARCHAR(50)",
                    },
                  ],
                  dbSpecific: {
                    fields: [],
                  },
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "createTable",
                createTable: {
                  name: "stock",
                  tableIndexes: [],
                  columns: [
                    {
                      name: "s_i_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                      constraint: "REFERENCES item(i_id)",
                    },
                    {
                      name: "s_w_id",
                      sqlType: "INTEGER",
                      primaryKey: true,
                      constraint: "REFERENCES warehouse(w_id)",
                    },
                    {
                      name: "s_quantity",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "s_dist_01",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_dist_02",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_dist_03",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_dist_04",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_dist_05",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_dist_06",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_dist_07",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_dist_08",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_dist_09",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_dist_10",
                      sqlType: "CHAR(24)",
                    },
                    {
                      name: "s_ytd",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "s_order_cnt",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "s_remote_cnt",
                      sqlType: "INTEGER",
                    },
                    {
                      name: "s_data",
                      sqlType: "VARCHAR(50)",
                    },
                  ],
                  dbSpecific: {
                    fields: [],
                  },
                },
              },
            },
          },
        ],
      },
      {
        name: "create_stored_procedures",
        units: [
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "create_dbms_random",
                  sql: "CREATE OR REPLACE FUNCTION DBMS_RANDOM (INTEGER, INTEGER) RETURNS INTEGER AS $$
DECLARE
  start_int ALIAS FOR $1;
  end_int ALIAS FOR $2;
BEGIN
  RETURN trunc(random() * (end_int-start_int + 1) + start_int);
END;
$$ LANGUAGE 'plpgsql' STRICT;
",
                  params: [],
                  groups: [],
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "create_neword_procedure",
                  sql: "CREATE OR REPLACE FUNCTION NEWORD (
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
",
                  params: [],
                  groups: [],
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "create_payment_procedure",
                  sql: "CREATE OR REPLACE FUNCTION PAYMENT (
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
",
                  params: [],
                  groups: [],
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "create_delivery_procedure",
                  sql: "CREATE OR REPLACE FUNCTION DELIVERY (
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
",
                  params: [],
                  groups: [],
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "create_ostat_procedure",
                  sql: "CREATE OR REPLACE FUNCTION OSTAT (
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
",
                  params: [],
                  groups: [],
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "create_slev_procedure",
                  sql: "CREATE OR REPLACE FUNCTION SLEV (
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
",
                  params: [],
                  groups: [],
                },
              },
            },
          },
        ],
      },
      {
        name: "load_data",
        units: [
          {
            count: String(ITEMS),
            descriptor: {
              type: {
                oneofKind: "insert",
                insert: {
                  name: "load_items",
                  tableName: "item",
                  params: [
                    {
                      name: "i_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: ITEMS,
                            min: 1,
                          },
                        },
                        unique: true,
                      },
                    },
                    {
                      name: "i_im_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: ITEMS,
                            min: 1,
                          },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
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
                          floatRange: {
                            max: 100,
                            min: 1,
                          },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
                              ],
                            },
                            minLen: "26",
                          },
                        },
                      },
                    },
                  ],
                  groups: [],
                  method: 1,
                },
              },
            },
          },
          {
            count: String(WAREHOUSES),
            descriptor: {
              type: {
                oneofKind: "insert",
                insert: {
                  name: "load_warehouses",
                  tableName: "warehouse",
                  params: [
                    {
                      name: "w_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: WAREHOUSES,
                            min: 1,
                          },
                        },
                        unique: true,
                      },
                    },
                    {
                      name: "w_name",
                      generationRule: {
                        kind: {
                          oneofKind: "stringRange",
                          stringRange: {
                            maxLen: "10",
                            minLen: "6",
                          },
                        },
                      },
                    },
                    {
                      name: "w_street_1",
                      generationRule: {
                        kind: {
                          oneofKind: "stringRange",
                          stringRange: {
                            maxLen: "20",
                            minLen: "10",
                          },
                        },
                      },
                    },
                    {
                      name: "w_street_2",
                      generationRule: {
                        kind: {
                          oneofKind: "stringRange",
                          stringRange: {
                            maxLen: "20",
                            minLen: "10",
                          },
                        },
                      },
                    },
                    {
                      name: "w_city",
                      generationRule: {
                        kind: {
                          oneofKind: "stringRange",
                          stringRange: {
                            maxLen: "20",
                            minLen: "10",
                          },
                        },
                      },
                    },
                    {
                      name: "w_state",
                      generationRule: {
                        kind: {
                          oneofKind: "stringRange",
                          stringRange: {
                            maxLen: "2",
                            minLen: "2",
                          },
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
                            alphabet: {
                              ranges: [
                                {
                                  max: 57,
                                  min: 48,
                                },
                              ],
                            },
                            minLen: "9",
                          },
                        },
                      },
                    },
                    {
                      name: "w_tax",
                      generationRule: {
                        kind: {
                          oneofKind: "floatRange",
                          floatRange: {
                            max: 0.2,
                          },
                        },
                      },
                    },
                    {
                      name: "w_ytd",
                      generationRule: {
                        kind: {
                          oneofKind: "floatConst",
                          floatConst: 300000,
                        },
                      },
                    },
                  ],
                  groups: [],
                  method: 1,
                },
              },
            },
          },
          {
            count: String(TOTAL_DISTRICTS),
            descriptor: {
              type: {
                oneofKind: "insert",
                insert: {
                  name: "load_districts",
                  tableName: "district",
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
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
                            alphabet: {
                              ranges: [
                                {
                                  max: 90,
                                  min: 65,
                                },
                              ],
                            },
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
                            alphabet: {
                              ranges: [
                                {
                                  max: 57,
                                  min: 48,
                                },
                              ],
                            },
                            minLen: "9",
                          },
                        },
                      },
                    },
                    {
                      name: "d_tax",
                      generationRule: {
                        kind: {
                          oneofKind: "floatRange",
                          floatRange: {
                            max: 0.2,
                          },
                        },
                      },
                    },
                    {
                      name: "d_ytd",
                      generationRule: {
                        kind: {
                          oneofKind: "floatConst",
                          floatConst: 30000,
                        },
                      },
                    },
                    {
                      name: "d_next_o_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Const",
                          int32Const: 3001,
                        },
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
                              int32Range: {
                                max: WAREHOUSES,
                                min: 1,
                              },
                            },
                            unique: true,
                          },
                        },
                        {
                          name: "d_id",
                          generationRule: {
                            kind: {
                              oneofKind: "int32Range",
                              int32Range: {
                                max: DISTRICTS_PER_WAREHOUSE,
                                min: 1,
                              },
                            },
                            unique: true,
                          },
                        },
                      ],
                    },
                  ],
                  method: 1,
                },
              },
            },
          },
          {
            count: String(TOTAL_CUSTOMERS),
            descriptor: {
              type: {
                oneofKind: "insert",
                insert: {
                  name: "load_customers",
                  tableName: "customer",
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
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
                        kind: {
                          oneofKind: "stringConst",
                          stringConst: "OE",
                        },
                      },
                    },
                    {
                      name: "c_last",
                      generationRule: {
                        kind: {
                          oneofKind: "stringRange",
                          stringRange: {
                            maxLen: "16",
                            minLen: "6",
                          },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
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
                            alphabet: {
                              ranges: [
                                {
                                  max: 90,
                                  min: 65,
                                },
                              ],
                            },
                            minLen: "2",
                          },
                        },
                      },
                    },
                    {
                      name: "c_zip",
                      generationRule: {
                        kind: {
                          oneofKind: "stringConst",
                          stringConst: "123456789",
                        },
                      },
                    },
                    {
                      name: "c_phone",
                      generationRule: {
                        kind: {
                          oneofKind: "stringRange",
                          stringRange: {
                            maxLen: "16",
                            alphabet: {
                              ranges: [
                                {
                                  max: 57,
                                  min: 48,
                                },
                              ],
                            },
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
                            value: {
                              seconds: "1761545738",
                              nanos: 810290275,
                            },
                          },
                        },
                      },
                    },
                    {
                      name: "c_credit",
                      generationRule: {
                        kind: {
                          oneofKind: "stringConst",
                          stringConst: "GC",
                        },
                      },
                    },
                    {
                      name: "c_credit_lim",
                      generationRule: {
                        kind: {
                          oneofKind: "floatConst",
                          floatConst: 50000,
                        },
                      },
                    },
                    {
                      name: "c_discount",
                      generationRule: {
                        kind: {
                          oneofKind: "floatRange",
                          floatRange: {
                            max: 0.5,
                          },
                        },
                      },
                    },
                    {
                      name: "c_balance",
                      generationRule: {
                        kind: {
                          oneofKind: "floatConst",
                          floatConst: -10,
                        },
                      },
                    },
                    {
                      name: "c_ytd_payment",
                      generationRule: {
                        kind: {
                          oneofKind: "floatConst",
                          floatConst: 10,
                        },
                      },
                    },
                    {
                      name: "c_payment_cnt",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Const",
                          int32Const: 1,
                        },
                      },
                    },
                    {
                      name: "c_delivery_cnt",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Const",
                          int32Const: 0,
                        },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
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
                              int32Range: {
                                max: DISTRICTS_PER_WAREHOUSE,
                                min: 1,
                              },
                            },
                            unique: true,
                          },
                        },
                        {
                          name: "c_w_id",
                          generationRule: {
                            kind: {
                              oneofKind: "int32Range",
                              int32Range: {
                                max: WAREHOUSES,
                                min: 1,
                              },
                            },
                            unique: true,
                          },
                        },
                        {
                          name: "c_id",
                          generationRule: {
                            kind: {
                              oneofKind: "int32Range",
                              int32Range: {
                                max: CUSTOMERS_PER_DISTRICT,
                                min: 1,
                              },
                            },
                            unique: true,
                          },
                        },
                      ],
                    },
                  ],
                  method: 1,
                },
              },
            },
          },
          {
            count: String(TOTAL_STOCK),
            descriptor: {
              type: {
                oneofKind: "insert",
                insert: {
                  name: "load_stock",
                  tableName: "stock",
                  params: [
                    {
                      name: "s_quantity",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: 100,
                            min: 10,
                          },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
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
                        kind: {
                          oneofKind: "int32Const",
                          int32Const: 0,
                        },
                      },
                    },
                    {
                      name: "s_order_cnt",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Const",
                          int32Const: 0,
                        },
                      },
                    },
                    {
                      name: "s_remote_cnt",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Const",
                          int32Const: 0,
                        },
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
                                {
                                  max: 90,
                                  min: 65,
                                },
                                {
                                  max: 122,
                                  min: 97,
                                },
                                {
                                  max: 57,
                                  min: 48,
                                },
                                {
                                  max: 33,
                                  min: 32,
                                },
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
                              int32Range: {
                                max: ITEMS,
                                min: 1,
                              },
                            },
                            unique: true,
                          },
                        },
                        {
                          name: "s_w_id",
                          generationRule: {
                            kind: {
                              oneofKind: "int32Range",
                              int32Range: {
                                max: WAREHOUSES,
                                min: 1,
                              },
                            },
                            unique: true,
                          },
                        },
                      ],
                    },
                  ],
                  method: 1,
                },
              },
            },
          },
        ],
      },
      {
        name: "tpcc_workload",
        units: [
          {
            count: "45",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "call_neword",
                  sql: "SELECT NEWORD(${w_id}, ${max_w_id}, ${d_id}, ${c_id}, ${ol_cnt}, 0)",
                  params: [
                    {
                      name: "w_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: WAREHOUSES,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "max_w_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: DISTRICTS_PER_WAREHOUSE,
                            min: DISTRICTS_PER_WAREHOUSE,
                          },
                        },
                      },
                    },
                    {
                      name: "d_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: DISTRICTS_PER_WAREHOUSE,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "c_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: CUSTOMERS_PER_DISTRICT,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "ol_cnt",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: 15,
                            min: 5,
                          },
                        },
                      },
                    },
                  ],
                  groups: [],
                },
              },
            },
          },
          {
            count: "43",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "payment_procedure",
                  sql: "SELECT PAYMENT(${p_w_id}, ${p_d_id}, ${p_c_w_id}, ${p_c_d_id},
${p_c_id}, ${byname}, ${h_amount}, ${c_last})",
                  params: [
                    {
                      name: "p_w_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: WAREHOUSES,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "p_d_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: DISTRICTS_PER_WAREHOUSE,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "p_c_w_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: WAREHOUSES,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "p_c_d_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: DISTRICTS_PER_WAREHOUSE,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "p_c_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: CUSTOMERS_PER_DISTRICT,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "byname",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: 0,
                            min: 0,
                          },
                        },
                      },
                    },
                    {
                      name: "h_amount",
                      generationRule: {
                        kind: {
                          oneofKind: "doubleRange",
                          doubleRange: {
                            max: 5000,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "c_last",
                      generationRule: {
                        kind: {
                          oneofKind: "stringRange",
                          stringRange: {
                            maxLen: "16",
                            minLen: "6",
                          },
                        },
                        unique: true,
                      },
                    },
                  ],
                  groups: [],
                },
              },
            },
          },
          {
            count: "4",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "order_status_procedure",
                  sql: "SELECT * FROM OSTAT(${os_w_id}, ${os_d_id}, ${os_c_id},
${byname}, ${os_c_last})",
                  params: [
                    {
                      name: "os_w_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: WAREHOUSES,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "os_d_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: DISTRICTS_PER_WAREHOUSE,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "os_c_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: CUSTOMERS_PER_DISTRICT,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "byname",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: 0,
                            min: 0,
                          },
                        },
                      },
                    },
                    {
                      name: "os_c_last",
                      generationRule: {
                        kind: {
                          oneofKind: "stringRange",
                          stringRange: {
                            maxLen: "16",
                            minLen: "8",
                          },
                        },
                      },
                    },
                  ],
                  groups: [],
                },
              },
            },
          },
          {
            count: "4",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "delivery_procedure",
                  sql: "SELECT DELIVERY(${d_w_id}, ${d_o_carrier_id})
",
                  params: [
                    {
                      name: "d_w_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: WAREHOUSES,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "d_o_carrier_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: DISTRICTS_PER_WAREHOUSE,
                            min: 1,
                          },
                        },
                      },
                    },
                  ],
                  groups: [],
                },
              },
            },
          },
          {
            count: "4",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "stock_level_transaction",
                  sql: "SELECT SLEV(${st_w_id}, ${st_d_id}, ${threshold})
",
                  params: [
                    {
                      name: "st_w_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: WAREHOUSES,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "st_d_id",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: DISTRICTS_PER_WAREHOUSE,
                            min: 1,
                          },
                        },
                      },
                    },
                    {
                      name: "threshold",
                      generationRule: {
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: {
                            max: 20,
                            min: 10,
                          },
                        },
                      },
                    },
                  ],
                  groups: [],
                },
              },
            },
          },
        ],
        async: true,
      },
      {
        name: "cleanup",
        units: [
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "drop_procedures",
                  sql: "DROP FUNCTION IF EXISTS neword, payment, delivery, DBMS_RANDOM CASCADE",
                  params: [],
                  groups: [],
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "drop_tables",
                  sql: "DROP TABLE IF EXISTS order_line, orders, new_order, history, customer, district, stock, item, warehouse CASCADE",
                  params: [],
                  groups: [],
                },
              },
            },
          },
        ],
      },
    ],
  },
};
*/
