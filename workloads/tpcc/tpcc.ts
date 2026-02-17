import { Options } from "k6/options";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import { NotifyStep, Teardown } from "k6/x/stroppy";

import { Status, InsertMethod, DriverConfig_DriverType } from "./stroppy.pb.js";
import { NewGen, AB, R, Step, DriverX, S } from "./helpers.ts";
import { parse_sql_with_groups } from "./parse_sql.js";

const DURATION = __ENV.DURATION || "1h";
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

const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS =
  WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK = WAREHOUSES * ITEMS;

// Initialize driver with GlobalConfig
const driver = DriverX.fromConfig({
  driver: {
    url: __ENV.DRIVER_URL || "postgres://postgres:postgres@localhost:5432",
    driverType: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
    connectionType: { is: {oneofKind:"sharedPool", sharedPool: {sharedConnections: 100}}},
    dbSpecific: {
      fields: [],
    },
  },
});

const sections = parse_sql_with_groups(open(__SQL_FILE));

export function setup() {
  Step("create_schema", () => {
    sections["drop_schema"].forEach((query) => driver.runQuery(query, {}));
    sections["create_schema"].forEach((query) => driver.runQuery(query, {}));
  });

  Step("load_data", () => {
    // Load data into tables using InsertValues with COPY_FROM method
    driver.insert("item", ITEMS, {
      method: InsertMethod.COPY_FROM,
      params: {
        i_id: S.int32(1, ITEMS),
        i_im_id: S.int32(1, ITEMS), // WHY: not unique originaly
        i_name: R.str(14, 24, AB.enSpc),
        i_price: R.float(1, 100),
        i_data: R.str(26, 50, AB.enSpc),
      },
    });

    driver.insert("warehouse", WAREHOUSES, {
      method: InsertMethod.COPY_FROM,
      params: {
        w_id: S.int32(1, WAREHOUSES),
        w_name: R.str(6, 10),
        w_street_1: R.str(10, 20),
        w_street_2: R.str(10, 20),
        w_city: R.str(10, 20),
        w_state: R.str(2),
        w_zip: R.str(9, AB.num),
        w_tax: R.float(0, 0.2),
        w_ytd: R.float(300000),
      },
    });

    driver.insert("district", TOTAL_DISTRICTS, {
      method: InsertMethod.COPY_FROM,
      params: {
        d_name: R.str(6, 10),
        d_street_1: R.str(10, 20, AB.enSpc),
        d_street_2: R.str(10, 20, AB.enSpc),
        d_city: R.str(10, 20, AB.enSpc),
        d_state: R.str(2, AB.enUpper),
        d_zip: R.str(9, AB.num),
        d_tax: R.float(0, 0.2),
        d_ytd: R.float(30000),
        d_next_o_id: R.int32(3001),
      },
      groups: {
        district_pk: {
          d_w_id: S.int32(1, WAREHOUSES),
          d_id: S.int32(1, DISTRICTS_PER_WAREHOUSE),
        },
      },
    });

    driver.insert("customer", TOTAL_CUSTOMERS, {
      method: InsertMethod.COPY_FROM,
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
        c_since: R.datetimeConst(new Date()),
        c_credit: R.str("GC"), // TODO: "GC" | "BC" (good/bad credit), and 10% should be "BC"
        c_credit_lim: R.float(50000),
        c_discount: R.float(0, 0.5),
        c_balance: R.float(-10),
        c_ytd_payment: R.float(10),
        c_payment_cnt: R.int32(1),
        c_delivery_cnt: R.int32(0),
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
      method: InsertMethod.COPY_FROM,
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
        s_ytd: R.int32(0),
        s_order_cnt: R.int32(0),
        s_remote_cnt: R.int32(0),
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

  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}

const newOrderWarehouseGen = NewGen(0, R.int32(1, WAREHOUSES));
const newOrderMaxWarehouseGen = NewGen(1, R.int32(WAREHOUSES));
const newOrderDistrictGen = NewGen(2, R.int32(1, DISTRICTS_PER_WAREHOUSE));
const newOrderCustomerGen = NewGen(3, R.int32(1, CUSTOMERS_PER_DISTRICT));
const newOrderOlCntGen = NewGen(4, R.int32(5, 15));
export function new_order() {
  driver.runQuery("SELECT NEWORD(:w_id, :max_w_id, :d_id, :c_id, :ol_cnt, 0)", {
    w_id: newOrderWarehouseGen.next(),
    max_w_id: newOrderMaxWarehouseGen.next(),
    d_id: newOrderDistrictGen.next(),
    c_id: newOrderCustomerGen.next(),
    ol_cnt: newOrderOlCntGen.next(),
  });
}

const paymentWarehouseGen = NewGen(5, R.int32(1, WAREHOUSES));
const paymentDistrictGen = NewGen(6, R.int32(1, DISTRICTS_PER_WAREHOUSE));
const paymentCustomerWarehouseGen = NewGen(7, R.int32(1, WAREHOUSES));
const paymentCustomerDistrictGen = NewGen(
  8,
  R.int32(1, DISTRICTS_PER_WAREHOUSE),
);
const paymentCustomerGen = NewGen(9, R.int32(1, CUSTOMERS_PER_DISTRICT));
const paymentAmountGen = NewGen(10, R.double(1, 5000));
const paymentCustomerLastGen = NewGen(11, S.str(6, 16));
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

const orderStatusWarehouseGen = NewGen(12, R.int32(1, WAREHOUSES));
const orderStatusDistrictGen = NewGen(13, R.int32(1, DISTRICTS_PER_WAREHOUSE));
const orderStatusCustomerGen = NewGen(14, R.int32(1, CUSTOMERS_PER_DISTRICT));
const orderStatusCustomerLastGen = NewGen(15, R.str(8, 16));
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

const deliveryWarehouseGen = NewGen(16, R.int32(1, WAREHOUSES));
const deliveryCarrierGen = NewGen(17, R.int32(1, DISTRICTS_PER_WAREHOUSE));
export function delivery() {
  driver.runQuery("SELECT DELIVERY(:d_w_id, :d_o_carrier_id)", {
    d_w_id: deliveryWarehouseGen.next(),
    d_o_carrier_id: deliveryCarrierGen.next(),
  });
}

const stockLevelWarehouseGen = NewGen(18, R.int32(1, WAREHOUSES));
const stockLevelDistrictGen = NewGen(19, R.int32(1, DISTRICTS_PER_WAREHOUSE));
const stockLevelThresholdGen = NewGen(20, R.int32(10, 20));
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
