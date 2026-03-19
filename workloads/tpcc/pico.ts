import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverConfig_DriverType } from "./stroppy.pb.js";
import { AB, C, R, Step, DriverX, S, ENV } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

const SQL_FILE = ENV("SQL_FILE", "./pico.sql", "Path to SQL file (automatically set if .sql file provided as argument)");
const DURATION = ENV("DURATION", "5m", "Test duration");
const VUS_SCALE = ENV("VUS_SCALE", 1, "VU scaling factor");

// TPCC Configuration Constants
const POOL_SIZE = ENV("POOL_SIZE", 1, "Connection pool size");
const WAREHOUSES = ENV(["SCALE_FACTOR", "WAREHOUSES"], 1, "Number of warehouses");
const DISTRICTS_PER_WAREHOUSE = 10;
const CUSTOMERS_PER_DISTRICT = 3000;
const ITEMS = 100000;

// Derived constants
const TOTAL_DISTRICTS = WAREHOUSES * DISTRICTS_PER_WAREHOUSE;
const TOTAL_CUSTOMERS =
  WAREHOUSES * DISTRICTS_PER_WAREHOUSE * CUSTOMERS_PER_DISTRICT;
const TOTAL_STOCK = WAREHOUSES * ITEMS;

export const options: Options = {
  setupTimeout: String(WAREHOUSES * 5) + "m",
  scenarios: {
    new_order: {
      executor: "constant-vus",
      exec: "new_order",
      vus: Math.max(1, Math.round(44 * VUS_SCALE)),
      duration: DURATION,
    },
    payments: {
      executor: "constant-vus",
      exec: "payments",
      vus: Math.max(1, Math.round(43 * VUS_SCALE)),
      duration: DURATION,
    },
    order_status: {
      executor: "constant-vus",
      exec: "order_status",
      vus: Math.max(1, Math.round(4 * VUS_SCALE)),
      duration: DURATION,
    },
    delivery: {
      executor: "constant-vus",
      exec: "delivery",
      vus: Math.max(1, Math.round(4 * VUS_SCALE)),
      duration: DURATION,
    },
    stock_level: {
      executor: "constant-vus",
      exec: "stock_level",
      vus: Math.max(1, Math.round(4 * VUS_SCALE)),
      duration: DURATION,
    },
  },
};

// Initialize driver — shared (created at init phase)
const driver = DriverX.create().setup({
  url: ENV("DRIVER_URL", "postgres://admin:T0psecret@localhost:1331", "Database connection URL"),
  driverType: "picodata",
  postgres: { maxConns: POOL_SIZE, minConns: POOL_SIZE },
});

const sql = parse_sql_with_sections(open(SQL_FILE));

export function setup() {
  Step("drop_schema", () => {
    sql("drop_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("create_schema", () => {
    sql("create_schema").forEach((query) => driver.exec(query, {}));
  });

  Step("load_data", () => {
    console.log("Loading item table...");
    driver.insert("item", ITEMS, {
      method: "plain_query",
      params: {
        i_id: S.int32(1, ITEMS),
        i_im_id: S.int32(1, ITEMS),
        i_name: R.str(14, 24, AB.enSpc),
        i_price: R.float(1, 100),
        i_data: R.str(26, 50, AB.enSpc),
      },
    });

    console.log("Loading warehouse table...");
    driver.insert("warehouse", WAREHOUSES, {
      method: "plain_query",
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

    console.log("Loading district table...");
    driver.insert("district", TOTAL_DISTRICTS, {
      method: "plain_query",
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

    console.log("Loading customer table...");
    driver.insert("customer", TOTAL_CUSTOMERS, {
      method: "plain_query",
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

    console.log("Loading stock table...");
    driver.insert("stock", TOTAL_STOCK, {
      method: "plain_query",
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

    console.log("Data loading completed!");
  });

  Step.begin("workload");
  return;
}

// ============================================================================
// NEWORD - New Order Transaction
// ============================================================================
const newordWIdGen = R.int32(1, WAREHOUSES).gen();
const newordDIdGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const newordCIdGen = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const newordOOlCntGen = R.int32(5, 15).gen();
const newordDNextOIdGen = R.int32(1, 100000).gen();
const newordItemIdGen = R.int32(1, ITEMS).gen();
const newordQuantityGen = R.int32(1, 10).gen();
const newordSQuantityGen = R.int32(10, 100).gen();
const newordDistInfoGen = R.str(24, AB.enNum).gen();
const newordAmountGen = R.double(1, 10000).gen();

export function new_order() {
  const no_w_id = newordWIdGen.next();
  const no_d_id = newordDIdGen.next();
  const no_c_id = newordCIdGen.next();
  const no_o_ol_cnt = newordOOlCntGen.next();
  const no_d_next_o_id = newordDNextOIdGen.next();
  const no_o_all_local = 1;

  // BUG: https://git.picodata.io/core/picodata/-/issues/2659
  // driver.exec(sql("workload", "neword_get_customer_warehouse")!, {
  //   w_id: no_w_id,
  //   d_id: no_d_id,
  //   c_id: no_c_id,
  // });

  driver.exec(sql("workload", "neword_get_district")!, {
    d_id: no_d_id,
    w_id: no_w_id,
  });

  driver.exec(sql("workload", "neword_update_district")!, {
    d_id: no_d_id,
    w_id: no_w_id,
  });

  // BUG: https://git.picodata.io/core/picodata/-/issues/2659
  driver.exec(sql("workload", "neword_insert_order")!, {
    o_id: no_d_next_o_id,
    d_id: no_d_id,
    w_id: no_w_id,
    c_id: no_c_id,
    ol_cnt: no_o_ol_cnt,
    all_local: no_o_all_local,
  });

  driver.exec(sql("workload","neword_insert_new_order")!, {
    o_id: no_d_next_o_id,
    d_id: no_d_id,
    w_id: no_w_id,
  });

  // PG: FOR loop_counter IN 1 .. no_o_ol_cnt LOOP
  for (let loop_counter = 1; loop_counter <= no_o_ol_cnt; loop_counter++) {
    // PG: item_id_array[loop_counter] := round(DBMS_RANDOM(1,100000))
    const item_id = newordItemIdGen.next();
    // PG: quantity_array[loop_counter] := round(DBMS_RANDOM(1,10))
    const quantity = newordQuantityGen.next();
    const no_s_quantity = newordSQuantityGen.next();
    const ol_dist_info = newordDistInfoGen.next();
    const ol_amount = newordAmountGen.next();

    driver.exec(sql("workload", "neword_get_item")!, {
      i_id: item_id,
    });

    driver.exec(sql("workload", "neword_get_stock")!, {
      i_id: item_id,
      w_id: no_w_id,
    });

    driver.exec(sql("workload", "neword_update_stock")!, {
      quantity: no_s_quantity,
      ol_quantity: quantity,
      remote_cnt: 0,
      i_id: item_id,
      w_id: no_w_id,
    });

    // BUG: https://git.picodata.io/core/picodata/-/issues/2659
    // driver.exec(sql("workload", "neword_insert_order_line")!, {
    //   o_id: no_d_next_o_id,
    //   d_id: no_d_id,
    //   w_id: no_w_id,
    //   ol_number: loop_counter,
    //   i_id: item_id,
    //   supply_w_id: no_w_id,
    //   quantity,
    //   amount: ol_amount,
    //   dist_info: ol_dist_info,
    // });
  }
}

// ============================================================================
// PAYMENT - Payment Transaction
// ============================================================================
const paymentWIdGen = R.int32(1, WAREHOUSES).gen();
const paymentDIdGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCWIdGen = R.int32(1, WAREHOUSES).gen();
const paymentCDIdGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCIdGen = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const paymentHAmountGen = R.double(1, 5000).gen();
const paymentCLastGen = S.str(6, 16).gen();
const paymentHDataGen = R.str(12, 24, AB.enSpc).gen();

export function payments() {
  const p_w_id = paymentWIdGen.next();
  const p_d_id = paymentDIdGen.next();
  const p_c_w_id = paymentCWIdGen.next();
  const p_c_d_id = paymentCDIdGen.next();
  const p_c_id = paymentCIdGen.next();
  const byname = 0;
  const p_h_amount = paymentHAmountGen.next();
  const p_c_last = paymentCLastGen.next();
  const h_data = paymentHDataGen.next();

  driver.exec(sql("workload", "payment_update_warehouse")!, {
    w_id: p_w_id,
    amount: p_h_amount,
  });

  driver.exec(sql("workload", "payment_get_warehouse")!, {
    w_id: p_w_id,
  });

  driver.exec(sql("workload", "payment_update_district")!, {
    w_id: p_w_id,
    d_id: p_d_id,
    amount: p_h_amount,
  });

  driver.exec(sql("workload", "payment_get_district")!, {
    w_id: p_w_id,
    d_id: p_d_id,
  });

  driver.exec(sql("workload", "payment_get_customer_by_id")!, {
    w_id: p_c_w_id,
    d_id: p_c_d_id,
    c_id: p_c_id,
  });

  driver.exec(sql("workload", "payment_update_customer")!, {
    w_id: p_c_w_id,
    d_id: p_c_d_id,
    c_id: p_c_id,
    amount: p_h_amount,
  });

  driver.exec(sql("workload", "payment_insert_history")!, {
    h_c_id: p_c_id,
    h_c_d_id: p_c_d_id,
    h_c_w_id: p_c_w_id,
    h_d_id: p_d_id,
    h_w_id: p_w_id,
    h_amount: p_h_amount,
    h_data,
  });
}

// ============================================================================
// OSTAT - Order Status Transaction
// ============================================================================
const ostatWIdGen = R.int32(1, WAREHOUSES).gen();
const ostatDIdGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const ostatCIdGen = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const ostatCLastGen = R.str(8, 16).gen();
const ostatOIdGen = R.int32(1, 100000).gen();

export function order_status() {
  const os_w_id = ostatWIdGen.next();
  const os_d_id = ostatDIdGen.next();
  const os_c_id = ostatCIdGen.next();
  const byname = 0;
  const os_c_last = ostatCLastGen.next();
  const os_o_id = ostatOIdGen.next();

  driver.exec(sql("workload", "ostat_get_customer_by_id")!, {
    c_id: os_c_id,
    d_id: os_d_id,
    w_id: os_w_id,
  });

  driver.exec(sql("workload", "ostat_get_last_order")!, {
    d_id: os_d_id,
    w_id: os_w_id,
    c_id: os_c_id,
  });

  driver.exec(sql("workload", "ostat_get_order_lines")!, {
    o_id: os_o_id,
    d_id: os_d_id,
    w_id: os_w_id,
  });
}

// ============================================================================
// DELIVERY - Delivery Transaction
// ============================================================================
const deliveryWIdGen = R.int32(1, WAREHOUSES).gen();
const deliveryOCarrierIdGen = R.int32(1, 10).gen();
const deliveryDateGen = C.datetime(new Date()).gen();
const deliveryNoOIdGen = R.int32(1, 100000).gen();
const deliveryCIdGen = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const deliveryOlTotalGen = R.double(1, 10000).gen();

export function delivery() {
  const d_w_id = deliveryWIdGen.next();
  const d_o_carrier_id = deliveryOCarrierIdGen.next();
  const delivery_d = deliveryDateGen.next();

  for (let d_id = 1; d_id <= DISTRICTS_PER_WAREHOUSE; d_id++) {
    const d_no_o_id = deliveryNoOIdGen.next();
    const d_c_id = deliveryCIdGen.next();
    const d_ol_total = deliveryOlTotalGen.next();

    driver.exec(sql("workload", "delivery_get_min_new_order")!, {
      d_id,
      w_id: d_w_id,
    });

    driver.exec(sql("workload", "delivery_delete_new_order")!, {
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    driver.exec(sql("workload", "delivery_get_order")!, {
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    driver.exec(sql("workload", "delivery_update_order")!, {
      carrier_id: d_o_carrier_id,
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    driver.exec(sql("workload", "delivery_update_order_line")!, {
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    driver.exec(sql("workload", "delivery_get_order_line_amount")!, {
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    driver.exec(sql("workload", "delivery_update_customer")!, {
      amount: d_ol_total,
      c_id: d_c_id,
      d_id,
      w_id: d_w_id,
    });
  }
}

// ============================================================================
// SLEV - Stock Level Transaction
// ============================================================================
const slevWIdGen = R.int32(1, WAREHOUSES).gen();
const slevDIdGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const slevThresholdGen = R.int32(10, 20).gen();
const slevNextOIdGen = R.int32(3001, 100000).gen();

export function stock_level() {
  const st_w_id = slevWIdGen.next();
  const st_d_id = slevDIdGen.next();
  const threshold = slevThresholdGen.next();
  const st_next_o_id = slevNextOIdGen.next();

  driver.exec(sql("workload", "slev_get_district")!, {
    w_id: st_w_id,
    d_id: st_d_id,
  });

  driver.exec(sql("workload", "slev_stock_count")!, {
    w_id: st_w_id,
    d_id: st_d_id,
    next_o_id: st_next_o_id,
    min_o_id: st_next_o_id - 20,
    threshold,
  });
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
