import { Options } from "k6/options";
import encoding from "k6/x/encoding";
globalThis.TextEncoder = encoding.TextEncoder;
globalThis.TextDecoder = encoding.TextDecoder;

import { NotifyStep, Teardown } from "k6/x/stroppy";

import { Status, InsertMethod } from "./stroppy.pb.js";
import {
  NewDriverByConfig,
  NewGeneratorByRule as NewGenByRule,
  AB,
  G,
  InsertValues,
} from "./helpers.ts";
import { parse_sql_with_groups } from "./parse_sql.js";

const DURATION = __ENV.DURATION || "1m";
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
// Wrapped in try-catch to handle teardown phase when DB may be unavailable
let driver: ReturnType<typeof NewDriverByConfig> | null = null;
try {
  driver = NewDriverByConfig({
    driver: {
      url: __ENV.DRIVER_URL || "postgres://admin:T0psecret@localhost:1331",
      driverType: 2,
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
} catch (e) {
  console.warn("Driver initialization failed (expected during teardown):", e);
}

const sections = parse_sql_with_groups(open(__SQL_FILE));

// Helper to get query SQL by name from workload section
function getQuery(name: string): string {
  const queries = sections["workload"] as any[];
  for (const q of queries) {
    if (q.name === name) {
      return q.sql;
    }
  }
  throw new Error(`Query not found: ${name}`);
}

// Helper to safely get rows from query result
function getRows(result: any): any[][] {
  if (result && result.rows) {
    return result.rows;
  }
  return [];
}

export function setup() {
  if (!driver) {
    throw new Error("Driver not initialized");
  }
  NotifyStep("create_schema", Status.STATUS_RUNNING);
  sections["drop_schema"].forEach((query) => driver.runQuery(query.sql, {}));
  sections["create_schema"].forEach((query) => driver.runQuery(query.sql, {}));
  NotifyStep("create_schema", Status.STATUS_COMPLETED);

  NotifyStep("load_data", Status.STATUS_RUNNING);
  // Load data into tables using InsertValues with COPY_FROM method
  console.log("Loading item table...");
  InsertValues(driver!, {
    count: ITEMS,
    tableName: "item",
    method: InsertMethod.PLAIN_QUERY,
    params: G.params({
      i_id: G.int32Seq(1, ITEMS),
      i_im_id: G.int32Seq(1, ITEMS), // WHY: not unique originaly
      i_name: G.str(14, 24, AB.enSpc),
      i_price: G.float(1, 100),
      i_data: G.str(26, 50, AB.enSpc),
    }),
    groups: [],
  });

  console.log("Loading warehouse table...");
  InsertValues(driver!, {
    count: WAREHOUSES,
    tableName: "warehouse",
    method: InsertMethod.PLAIN_QUERY,
    params: G.params({
      w_id: G.int32Seq(1, WAREHOUSES),
      w_name: G.str(6, 10),
      w_street_1: G.str(10, 20),
      w_street_2: G.str(10, 20),
      w_city: G.str(10, 20),
      w_state: G.str(2),
      w_zip: G.str(9, AB.num),
      w_tax: G.float(0, 0.2),
      w_ytd: G.float(300000),
    }),
    groups: [],
  });

  console.log("Loading district table...");
  InsertValues(driver!, {
    count: TOTAL_DISTRICTS,
    tableName: "district",
    method: InsertMethod.PLAIN_QUERY,
    params: G.params({
      d_name: G.str(6, 10),
      d_street_1: G.str(10, 20, AB.enSpc),
      d_street_2: G.str(10, 20, AB.enSpc),
      d_city: G.str(10, 20, AB.enSpc),
      d_state: G.str(2, AB.enUpper),
      d_zip: G.str(9, AB.num),
      d_tax: G.float(0, 0.2),
      d_ytd: G.float(30000),
      d_next_o_id: G.int32(3001),
    }),
    groups: G.groups({
      district_pk: G.params({
        d_w_id: G.int32Seq(1, WAREHOUSES),
        d_id: G.int32Seq(1, DISTRICTS_PER_WAREHOUSE),
      }),
    }),
  });

  console.log("Loading customer table...");
  InsertValues(driver!, {
    count: TOTAL_CUSTOMERS,
    tableName: "customer",
    method: InsertMethod.PLAIN_QUERY,
    params: G.params({
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
    }),
    groups: G.groups({
      customer_pk: G.params({
        c_d_id: G.int32Seq(1, DISTRICTS_PER_WAREHOUSE),
        c_w_id: G.int32Seq(1, WAREHOUSES),
        c_id: G.int32Seq(1, CUSTOMERS_PER_DISTRICT),
      }),
    }),
  });

  console.log("Loading stock table...");
  InsertValues(driver!, {
    count: TOTAL_STOCK,
    tableName: "stock",
    method: InsertMethod.PLAIN_QUERY,
    params: G.params({
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
    }),
    groups: G.groups({
      stock_pk: G.params({
        s_i_id: G.int32Seq(1, ITEMS),
        s_w_id: G.int32Seq(1, WAREHOUSES),
      }),
    }),
  });

  console.log("Data loading completed!");
  NotifyStep("load_data", Status.STATUS_COMPLETED);

  NotifyStep("workload", Status.STATUS_RUNNING);
  return;
}



// New Order Transaction
const newOrderWarehouseGen = NewGenByRule(0, G.int32(1, WAREHOUSES));
const newOrderDistrictGen = NewGenByRule(2, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const newOrderCustomerGen = NewGenByRule(3, G.int32(1, CUSTOMERS_PER_DISTRICT));
const newOrderOlCntGen = NewGenByRule(4, G.int32(5, 15));
const newOrderItemGen = NewGenByRule(5, G.int32(1, ITEMS));
const newOrderQuantityGen = NewGenByRule(6, G.int32(1, 10));
const newOrderEntryDateGen = NewGenByRule(7, G.datetimeConst(new Date()));
const newOrderDistInfoGen = NewGenByRule(8, G.str(24, AB.enNum));

export function new_order() {
  const w_id = newOrderWarehouseGen.next();
  const d_id = newOrderDistrictGen.next();
  const c_id = newOrderCustomerGen.next();
  const ol_cnt = newOrderOlCntGen.next();

  // Get customer and warehouse info
  driver!.runQuery(getQuery("neword_get_customer_warehouse"), {
    w_id,
    d_id,
    c_id,
  });

  // Get district info and next order ID
  const districtResult = driver!.runQuery(getQuery("neword_get_district"), {
    d_id,
    w_id,
  });
  const districtRows = getRows(districtResult);
  const d_next_o_id = districtRows[0]?.[0] || 3001;

  // Update district next order ID
  driver!.runQuery(getQuery("neword_update_district"), { d_id, w_id });

  const o_id = d_next_o_id;
  const entry_d = newOrderEntryDateGen.next();

  // // Insert order
  driver!.runQuery(getQuery("neword_insert_order"), {
    o_id,
    d_id,
    w_id,
    c_id,
    entry_d,
    ol_cnt,
    all_local: 1,
  });

  // Insert new order
  // driver!.runQuery(getQuery("neword_insert_new_order"), { o_id, d_id, w_id });

  // Process order lines
  for (let ol_number = 1; ol_number <= ol_cnt; ol_number++) {
    const i_id = newOrderItemGen.next();
    const ol_quantity = newOrderQuantityGen.next();
    const supply_w_id = w_id;

    // Get item info
    const itemResult = driver!.runQuery(getQuery("neword_get_item"), { i_id });
    const itemRows = getRows(itemResult);
    const i_price = itemRows[0]?.[0] || 10.0;

    // Get stock info
    const stockResult = driver!.runQuery(getQuery("neword_get_stock"), {
      i_id,
      w_id,
    });
    const stockRows = getRows(stockResult);
    const s_quantity = stockRows[0]?.[0] || 100;
    const s_dist = stockRows[0]?.[d_id + 1] || newOrderDistInfoGen.next(); // s_dist_01 to s_dist_10

    // Update stock
    const new_quantity = s_quantity >= ol_quantity + 10 ? s_quantity - ol_quantity : s_quantity - ol_quantity + 91;
    driver!.runQuery(getQuery("neword_update_stock"), {
      quantity: new_quantity,
      ol_quantity,
      remote_cnt: 0,
      i_id,
      w_id,
    });

    // Insert order line
    const ol_amount = ol_quantity * i_price;
    // driver!.runQuery(getQuery("neword_insert_order_line"), {
    //   o_id,
    //   d_id,
    //   w_id,
    //   ol_number,
    //   i_id,
    //   supply_w_id,
    //   quantity: ol_quantity,
    //   amount: ol_amount,
    //   dist_info: s_dist,
    // });
  }
}

// Payment Transaction
const paymentWarehouseGen = NewGenByRule(10, G.int32(1, WAREHOUSES));
const paymentDistrictGen = NewGenByRule(11, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const paymentCustomerWarehouseGen = NewGenByRule(12, G.int32(1, WAREHOUSES));
const paymentCustomerDistrictGen = NewGenByRule(13, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const paymentCustomerGen = NewGenByRule(14, G.int32(1, CUSTOMERS_PER_DISTRICT));
const paymentAmountGen = NewGenByRule(15, G.double(1, 5000));
const paymentDateGen = NewGenByRule(16, G.datetimeConst(new Date()));
const paymentDataGen = NewGenByRule(17, G.str(12, 24, AB.enSpc));

export function payments() {
  const w_id = paymentWarehouseGen.next();
  const d_id = paymentDistrictGen.next();
  const c_w_id = paymentCustomerWarehouseGen.next();
  const c_d_id = paymentCustomerDistrictGen.next();
  const c_id = paymentCustomerGen.next();
  const amount = paymentAmountGen.next();

  // Update warehouse YTD
  driver!.runQuery(getQuery("payment_update_warehouse"), { amount, w_id });

  // Get warehouse info
  const warehouseResult = driver!.runQuery(getQuery("payment_get_warehouse"), { w_id });
  const warehouseRows = getRows(warehouseResult);
  const w_name = warehouseRows[0]?.[0] || "";

  // Update district YTD
  driver!.runQuery(getQuery("payment_update_district"), { amount, w_id, d_id });

  // Get district info
  const payDistrictResult = driver!.runQuery(getQuery("payment_get_district"), { w_id, d_id });
  const payDistrictRows = getRows(payDistrictResult);
  const d_name = payDistrictRows[0]?.[0] || "";

  // Get customer by ID (simplified - not using byname lookup)
  driver!.runQuery(getQuery("payment_get_customer_by_id"), {
    w_id: c_w_id,
    d_id: c_d_id,
    c_id,
  });

  // Update customer
  driver!.runQuery(getQuery("payment_update_customer"), {
    amount,
    w_id: c_w_id,
    d_id: c_d_id,
    c_id,
  });

  // Insert history
  const h_date = paymentDateGen.next();
  const h_data = w_name && d_name ? `${w_name}    ${d_name}` : paymentDataGen.next();
  // driver!.runQuery(getQuery("payment_insert_history"), {
  //   c_d_id,
  //   c_w_id,
  //   c_id,
  //   d_id,
  //   w_id,
  //   h_date,
  //   amount,
  //   h_data,
  // });
}

// Order Status Transaction
const orderStatusWarehouseGen = NewGenByRule(20, G.int32(1, WAREHOUSES));
const orderStatusDistrictGen = NewGenByRule(21, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const orderStatusCustomerGen = NewGenByRule(22, G.int32(1, CUSTOMERS_PER_DISTRICT));

export function order_status() {
  const w_id = orderStatusWarehouseGen.next();
  const d_id = orderStatusDistrictGen.next();
  const c_id = orderStatusCustomerGen.next();

  // Get customer by ID
  driver!.runQuery(getQuery("ostat_get_customer_by_id"), { c_id, d_id, w_id });

  // Get last order
  const orderResult = driver!.runQuery(getQuery("ostat_get_last_order"), {
    d_id,
    w_id,
    c_id,
  });
  const orderRows = getRows(orderResult);
  const o_id = orderRows[0]?.[0];

  if (o_id) {
    // Get order lines
    driver!.runQuery(getQuery("ostat_get_order_lines"), { o_id, d_id, w_id });
  }
}

// Delivery Transaction
const deliveryWarehouseGen = NewGenByRule(25, G.int32(1, WAREHOUSES));
const deliveryCarrierGen = NewGenByRule(26, G.int32(1, 10));
const deliveryDateGen = NewGenByRule(27, G.datetimeConst(new Date()));

export function delivery() {
  const w_id = deliveryWarehouseGen.next();
  const carrier_id = deliveryCarrierGen.next();
  const delivery_d = deliveryDateGen.next();

  // Process each district
  for (let d_id = 1; d_id <= DISTRICTS_PER_WAREHOUSE; d_id++) {
    // Get minimum new order ID
    const newOrderResult = driver!.runQuery(getQuery("delivery_get_min_new_order"), {
      d_id,
      w_id,
    });
    const newOrderRows = getRows(newOrderResult);
    const o_id = newOrderRows[0]?.[0];

    if (!o_id) continue;

    // Delete new order
    driver!.runQuery(getQuery("delivery_delete_new_order"), { o_id, d_id, w_id });

    // Get order customer ID
    const delOrderResult = driver!.runQuery(getQuery("delivery_get_order"), {
      o_id,
      d_id,
      w_id,
    });
    const delOrderRows = getRows(delOrderResult);
    const c_id = delOrderRows[0]?.[0];

    // Update order with carrier
    driver!.runQuery(getQuery("delivery_update_order"), {
      carrier_id,
      o_id,
      d_id,
      w_id,
    });

    // Update order lines with delivery date
    driver!.runQuery(getQuery("delivery_update_order_line"), {
      delivery_d,
      o_id,
      d_id,
      w_id,
    });

    // Get order line amount sum
    const amountResult = driver!.runQuery(getQuery("delivery_get_order_line_amount"), {
      o_id,
      d_id,
      w_id,
    });
    const amountRows = getRows(amountResult);
    const amount = amountRows[0]?.[0] || 0;

    // Update customer balance
    if (c_id) {
      driver!.runQuery(getQuery("delivery_update_customer"), {
        amount,
        c_id,
        d_id,
        w_id,
      });
    }
  }
}

// Stock Level Transaction
const stockLevelWarehouseGen = NewGenByRule(30, G.int32(1, WAREHOUSES));
const stockLevelDistrictGen = NewGenByRule(31, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const stockLevelThresholdGen = NewGenByRule(32, G.int32(10, 20));

export function stock_level() {
  const w_id = stockLevelWarehouseGen.next();
  const d_id = stockLevelDistrictGen.next();
  const threshold = stockLevelThresholdGen.next();

  // Get district next order ID
  const slevDistrictResult = driver!.runQuery(getQuery("slev_get_district"), {
    w_id,
    d_id,
  });
  const slevDistrictRows = getRows(slevDistrictResult);
  const next_o_id = slevDistrictRows[0]?.[0] || 3001;
  const min_o_id = next_o_id - 20;

  // Count low stock items
  // driver!.runQuery(getQuery("slev_stock_count"), {
  //   w_id,
  //   d_id,
  //   next_o_id,
  //   min_o_id,
  //   threshold,
  // });
}

export function teardown() {
  NotifyStep("workload", Status.STATUS_COMPLETED);
  Teardown();
}
