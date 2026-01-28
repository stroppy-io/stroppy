import { Options } from "k6/options";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import { NotifyStep, Teardown } from "k6/x/stroppy";

import { Status, InsertMethod, DriverConfig_DriverType } from "./stroppy.pb.js";
import {
  NewDriverByConfig,
  NewGeneratorByRule as NewGenByRule,
  AB,
  G,
  InsertValues,
  StepBlock,
} from "./helpers.ts";
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

// Derived constants
const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS =
  WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK = WAREHOUSES * ITEMS;

// Initialize driver with GlobalConfig
const driver = NewDriverByConfig({
  driver: {
    url: __ENV.DRIVER_URL || "postgres://postgres:postgres@localhost:5432",
    driverType: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
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
});

const sections = parse_sql_with_groups(open(__SQL_FILE));

export function setup() {
  StepBlock("create_schema", () => {
    sections["drop_schema"].forEach((query) => driver.runQuery(query.sql, {}));
    sections["create_schema"].forEach((query) =>
      driver.runQuery(query.sql, {}),
    );
  });

  StepBlock("load_data", () => {
    // Load data into tables using InsertValues with COPY_FROM method
    InsertValues(driver, "item", ITEMS, {
      method: InsertMethod.COPY_FROM,
      params: {
        i_id: G.int32Seq(1, ITEMS),
        i_im_id: G.int32Seq(1, ITEMS), // WHY: not unique originaly
        i_name: G.str(14, 24, AB.enSpc),
        i_price: G.float(1, 100),
        i_data: G.str(26, 50, AB.enSpc),
      },
    });

    InsertValues(driver, "warehouse", WAREHOUSES, {
      method: InsertMethod.COPY_FROM,
      params: {
        w_id: G.int32Seq(1, WAREHOUSES),
        w_name: G.str(6, 10),
        w_street_1: G.str(10, 20),
        w_street_2: G.str(10, 20),
        w_city: G.str(10, 20),
        w_state: G.str(2),
        w_zip: G.str(9, AB.num),
        w_tax: G.float(0, 0.2),
        w_ytd: G.float(300000),
      },
    });

    InsertValues(driver, "district", TOTAL_DISTRICTS, {
      method: InsertMethod.COPY_FROM,
      params: {
        d_name: G.str(6, 10),
        d_street_1: G.str(10, 20, AB.enSpc),
        d_street_2: G.str(10, 20, AB.enSpc),
        d_city: G.str(10, 20, AB.enSpc),
        d_state: G.str(2, AB.enUpper),
        d_zip: G.str(9, AB.num),
        d_tax: G.float(0, 0.2),
        d_ytd: G.float(30000),
        d_next_o_id: G.int32(3001),
      },
      groups: {
        district_pk: G.params({
          d_w_id: G.int32Seq(1, WAREHOUSES),
          d_id: G.int32Seq(1, DISTRICTS_PER_WAREHOUSE),
        }),
      },
    });

    InsertValues(driver, "customer", TOTAL_CUSTOMERS, {
      method: InsertMethod.COPY_FROM,
      params: {
        c_first: G.str(8, 16),
        c_middle: G.str(2, AB.enUpper),
        c_last: G.strSeq(6, 16),
        c_street_1: G.str(10, 20, AB.enNumSpc),
        c_street_2: G.str(10, 20, AB.enNumSpc),
        c_city: G.str(10, 20, AB.enSpc),
        c_state: G.str(2, AB.enUpper),
        c_zip: G.str(9, AB.num),
        c_phone: G.str(16, AB.num),
        c_since: G.datetimeConst(new Date()),
        c_credit: G.str("GC"), // TODO: "GC" | "BC" (good/bad credit), and 10% should be "BC"
        c_credit_lim: G.float(50000),
        c_discount: G.float(0, 0.5),
        c_balance: G.float(-10),
        c_ytd_payment: G.float(10),
        c_payment_cnt: G.int32(1),
        c_delivery_cnt: G.int32(0),
        c_data: G.str(300, 500, AB.enNumSpc),
      },
      groups: {
        customer_pk: G.params({
          c_d_id: G.int32Seq(1, DISTRICTS_PER_WAREHOUSE),
          c_w_id: G.int32Seq(1, WAREHOUSES),
          c_id: G.int32Seq(1, CUSTOMERS_PER_DISTRICT),
        }),
      },
    });

    InsertValues(driver, "stock", TOTAL_STOCK, {
      method: InsertMethod.COPY_FROM,
      params: {
        s_quantity: G.int32(10, 100),
        s_dist_01: G.str(24, AB.enNum),
        s_dist_02: G.str(24, AB.enNum),
        s_dist_03: G.str(24, AB.enNum),
        s_dist_04: G.str(24, AB.enNum),
        s_dist_05: G.str(24, AB.enNum),
        s_dist_06: G.str(24, AB.enNum),
        s_dist_07: G.str(24, AB.enNum),
        s_dist_08: G.str(24, AB.enNum),
        s_dist_09: G.str(24, AB.enNum),
        s_dist_10: G.str(24, AB.enNum),
        s_ytd: G.int32(0),
        s_order_cnt: G.int32(0),
        s_remote_cnt: G.int32(0),
        s_data: G.str(26, 50, AB.enNumSpc),
      },
      groups: {
        stock_pk: G.params({
          s_i_id: G.int32Seq(1, ITEMS),
          s_w_id: G.int32Seq(1, WAREHOUSES),
        }),
      },
    });
  });

  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}

const newOrderWarehouseGen = NewGenByRule(0, G.int32(1, WAREHOUSES));
const newOrderMaxWarehouseGen = NewGenByRule(1, G.int32(WAREHOUSES));
const newOrderDistrictGen = NewGenByRule(
  2,
  G.int32(1, DISTRICTS_PER_WAREHOUSE),
);
const newOrderCustomerGen = NewGenByRule(3, G.int32(1, CUSTOMERS_PER_DISTRICT));
const newOrderOlCntGen = NewGenByRule(4, G.int32(5, 15));
export function new_order() {
  driver.runQuery("SELECT NEWORD(:w_id, :max_w_id, :d_id, :c_id, :ol_cnt, 0)", {
    w_id: newOrderWarehouseGen.next(),
    max_w_id: newOrderMaxWarehouseGen.next(),
    d_id: newOrderDistrictGen.next(),
    c_id: newOrderCustomerGen.next(),
    ol_cnt: newOrderOlCntGen.next(),
  });
}

const paymentWarehouseGen = NewGenByRule(5, G.int32(1, WAREHOUSES));
const paymentDistrictGen = NewGenByRule(6, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const paymentCustomerWarehouseGen = NewGenByRule(7, G.int32(1, WAREHOUSES));
const paymentCustomerDistrictGen = NewGenByRule(
  8,
  G.int32(1, DISTRICTS_PER_WAREHOUSE),
);
const paymentCustomerGen = NewGenByRule(9, G.int32(1, CUSTOMERS_PER_DISTRICT));
const paymentAmountGen = NewGenByRule(10, G.double(1, 5000));
const paymentCustomerLastGen = NewGenByRule(11, G.strSeq(6, 16));
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

const orderStatusWarehouseGen = NewGenByRule(12, G.int32(1, WAREHOUSES));
const orderStatusDistrictGen = NewGenByRule(
  13,
  G.int32(1, DISTRICTS_PER_WAREHOUSE),
);
const orderStatusCustomerGen = NewGenByRule(
  14,
  G.int32(1, CUSTOMERS_PER_DISTRICT),
);
const orderStatusCustomerLastGen = NewGenByRule(15, G.str(8, 16));
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

const deliveryWarehouseGen = NewGenByRule(16, G.int32(1, WAREHOUSES));
const deliveryCarrierGen = NewGenByRule(
  17,
  G.int32(1, DISTRICTS_PER_WAREHOUSE),
);
export function delivery() {
  driver.runQuery("SELECT DELIVERY(:d_w_id, :d_o_carrier_id)", {
    d_w_id: deliveryWarehouseGen.next(),
    d_o_carrier_id: deliveryCarrierGen.next(),
  });
}

const stockLevelWarehouseGen = NewGenByRule(18, G.int32(1, WAREHOUSES));
const stockLevelDistrictGen = NewGenByRule(
  19,
  G.int32(1, DISTRICTS_PER_WAREHOUSE),
);
const stockLevelThresholdGen = NewGenByRule(20, G.int32(10, 20));
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
