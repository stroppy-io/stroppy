import { Options } from "k6/options";
import { Teardown, NewPicker } from "k6/x/stroppy";
import { AB, C, R, Step, DriverX, S, ENV, declareDriverSetup } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

// TPC-C Configuration Constants
const POOL_SIZE   = ENV("POOL_SIZE", 100, "Connection pool size");
const WAREHOUSES  = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");

const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT  = 3000;
const ITEMS = 100000;

const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK     = WAREHOUSES * ITEMS;

// K6 options — weighted dispatch inside default(), VUs/duration set via CLI or k6 defaults.
export const options: Options = {
  setupTimeout: String(WAREHOUSES * 5) + "m",
};

// Driver config: defaults for postgres, overridable via CLI (--driver pg/mysql)
const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
  defaultInsertMethod: "copy_from",
  pool: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

// procs.ts targets pg + mysql only — picodata and ydb have no stored procedures.
if (driverConfig.driverType === "picodata" || driverConfig.driverType === "ydb") {
  throw new Error(
    `tpcc/procs.ts only supports postgres and mysql (got driverType=${driverConfig.driverType}). ` +
    `Use tpcc/tx.ts for picodata/ydb.`,
  );
}

const _sqlByDriver: Record<string, string> = {
  postgres: "./pg.sql",
  mysql:    "./mysql.sql",
};
const SQL_FILE = ENV("SQL_FILE", ENV.auto, "SQL file path (defaults per driverType)")
  ?? _sqlByDriver[driverConfig.driverType!]
  ?? "./pg.sql";

const driver = DriverX.create().setup(driverConfig);

const sql = parse_sql_with_sections(open(SQL_FILE));

// Per-VU monotonic counter for h_id. History table has a PRIMARY KEY on h_id
// across all dialects (for uniformity with tx.ts and picodata/ydb schemas).
// High offset (__VU * 10M) keeps VUs disjoint.
declare const __VU: number;
const _vu = (typeof __VU === "number" && __VU > 0) ? __VU : 1;
let hid_counter = _vu * 10_000_000;
const nextHid = (): number => ++hid_counter;

export function setup() {
  Step("drop_schema", () => {
    sql("drop_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_schema", () => {
    sql("create_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_procedures", () => {
    sql("create_procedures").forEach((query) => driver.exec(query, {}));
  });

  Step("load_data", () => {
    driver.insert("item", ITEMS, {
      params: {
        i_id: S.int32(1, ITEMS),
        i_im_id: S.int32(1, ITEMS),
        i_name: R.str(14, 24, AB.enSpc),
        i_price: R.float(1, 100),
        i_data: R.str(26, 50, AB.enSpc),
      },
    });

    driver.insert("warehouse", WAREHOUSES, {
      params: {
        w_id: S.int32(1, WAREHOUSES),
        w_name: R.str(6, 10),
        w_street_1: R.str(10, 20),
        w_street_2: R.str(10, 20),
        w_city: R.str(10, 20),
        w_state: R.str(2),
        w_zip: R.str(9, AB.num),
        w_tax: R.float(0, 0.2),
        w_ytd: C.float(300000),
      },
    });

    driver.insert("district", TOTAL_DISTRICTS, {
      params: {
        d_name: R.str(6, 10),
        d_street_1: R.str(10, 20, AB.enSpc),
        d_street_2: R.str(10, 20, AB.enSpc),
        d_city: R.str(10, 20, AB.enSpc),
        d_state: R.str(2, AB.enUpper),
        d_zip: R.str(9, AB.num),
        d_tax: R.float(0, 0.2),
        d_ytd: C.float(30000),
        d_next_o_id: C.int32(3001),
      },
      groups: {
        district_pk: {
          d_w_id: S.int32(1, WAREHOUSES),
          d_id: S.int32(1, DISTRICTS_PER_WAREHOUSE),
        },
      },
    });

    driver.insert("customer", TOTAL_CUSTOMERS, {
      params: {
        c_first: R.str(8, 16),
        c_middle: R.str(2, AB.enUpper),
        c_last: S.str(6, 16),
        c_street_1: R.str(10, 20, AB.enNumSpc),
        c_street_2: R.str(10, 20, AB.enNumSpc),
        c_city: R.str(10, 20, AB.enSpc),
        c_state: R.str(2, AB.enUpper),
        c_zip: R.str(9, AB.num),
        c_phone: R.str(16, AB.num),
        c_since: C.datetime(new Date()),
        c_credit: C.str("GC"),
        c_credit_lim: C.float(50000),
        c_discount: R.float(0, 0.5),
        c_balance: C.float(-10),
        c_ytd_payment: C.float(10),
        c_payment_cnt: C.int32(1),
        c_delivery_cnt: C.int32(0),
        c_data: R.str(300, 500, AB.enNumSpc),
      },
      groups: {
        customer_pk: {
          c_d_id: S.int32(1, DISTRICTS_PER_WAREHOUSE),
          c_w_id: S.int32(1, WAREHOUSES),
          c_id: S.int32(1, CUSTOMERS_PER_DISTRICT),
        },
      },
    });

    driver.insert("stock", TOTAL_STOCK, {
      params: {
        s_quantity: R.int32(10, 100),
        s_dist_01: R.str(24, AB.enNum),
        s_dist_02: R.str(24, AB.enNum),
        s_dist_03: R.str(24, AB.enNum),
        s_dist_04: R.str(24, AB.enNum),
        s_dist_05: R.str(24, AB.enNum),
        s_dist_06: R.str(24, AB.enNum),
        s_dist_07: R.str(24, AB.enNum),
        s_dist_08: R.str(24, AB.enNum),
        s_dist_09: R.str(24, AB.enNum),
        s_dist_10: R.str(24, AB.enNum),
        s_ytd: C.int32(0),
        s_order_cnt: C.int32(0),
        s_remote_cnt: C.int32(0),
        s_data: R.str(26, 50, AB.enNumSpc),
      },
      groups: {
        stock_pk: {
          s_i_id: S.int32(1, ITEMS),
          s_w_id: S.int32(1, WAREHOUSES),
        },
      },
    });
  });

  Step.begin("workload");
}

// =====================================================================
// Per-tx parameter generators (kept module-level for cheap reuse)
// =====================================================================

const newOrderWarehouseGen    = R.int32(1, WAREHOUSES).gen();
const newOrderMaxWarehouseGen = C.int32(WAREHOUSES).gen();
const newOrderDistrictGen     = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const newOrderCustomerGen     = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const newOrderOlCntGen        = R.int32(5, 15).gen();

function new_order() {
  driver.exec(sql("workload_procs", "new_order")!, {
    w_id: newOrderWarehouseGen.next(),
    max_w_id: newOrderMaxWarehouseGen.next(),
    d_id: newOrderDistrictGen.next(),
    c_id: newOrderCustomerGen.next(),
    ol_cnt: newOrderOlCntGen.next(),
  });
}

const paymentWarehouseGen         = R.int32(1, WAREHOUSES).gen();
const paymentDistrictGen          = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCustomerWarehouseGen = R.int32(1, WAREHOUSES).gen();
const paymentCustomerDistrictGen  = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCustomerGen          = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const paymentAmountGen            = R.double(1, 5000).gen();
const paymentCustomerLastGen      = S.str(6, 16).gen();

function payment() {
  driver.exec(sql("workload_procs", "payment")!, {
    p_w_id: paymentWarehouseGen.next(),
    p_d_id: paymentDistrictGen.next(),
    p_c_w_id: paymentCustomerWarehouseGen.next(),
    p_c_d_id: paymentCustomerDistrictGen.next(),
    p_c_id: paymentCustomerGen.next(),
    byname: 0,
    h_amount: paymentAmountGen.next(),
    c_last: paymentCustomerLastGen.next(),
    p_h_id: nextHid(),
  });
}

const orderStatusWarehouseGen    = R.int32(1, WAREHOUSES).gen();
const orderStatusDistrictGen     = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const orderStatusCustomerGen     = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const orderStatusCustomerLastGen = R.str(8, 16).gen();

function order_status() {
  driver.exec(sql("workload_procs", "order_status")!, {
    os_w_id: orderStatusWarehouseGen.next(),
    os_d_id: orderStatusDistrictGen.next(),
    os_c_id: orderStatusCustomerGen.next(),
    byname: 0,
    os_c_last: orderStatusCustomerLastGen.next(),
  });
}

const deliveryWarehouseGen = R.int32(1, WAREHOUSES).gen();
const deliveryCarrierGen   = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();

function delivery() {
  driver.exec(sql("workload_procs", "delivery")!, {
    d_w_id: deliveryWarehouseGen.next(),
    d_o_carrier_id: deliveryCarrierGen.next(),
  });
}

const stockLevelWarehouseGen = R.int32(1, WAREHOUSES).gen();
const stockLevelDistrictGen  = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const stockLevelThresholdGen = R.int32(10, 20).gen();

function stock_level() {
  driver.exec(sql("workload_procs", "stock_level")!, {
    st_w_id: stockLevelWarehouseGen.next(),
    st_d_id: stockLevelDistrictGen.next(),
    threshold: stockLevelThresholdGen.next(),
  });
}

// =====================================================================
// Weighted dispatch — TPC-C standard mix: 45/43/4/4/4 (sums to 100)
// =====================================================================
const picker = NewPicker(0);

export default function (): void {
  const workload = picker.pickWeighted(
    [new_order, payment, order_status, delivery, stock_level],
    [45,        43,      4,            4,        4],
  ) as () => void;
  workload();
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
