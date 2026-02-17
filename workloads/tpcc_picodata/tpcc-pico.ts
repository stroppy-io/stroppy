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
let driver = NewDriverByConfig({
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

export function setup() {
  if (!driver) {
    throw new Error("Driver not initialized");
  }
  NotifyStep("create_schema", Status.STATUS_RUNNING);
  sections["drop_schema"].forEach((query) => driver!.runQuery(query.sql, {}));
  sections["create_schema"].forEach((query) => driver!.runQuery(query.sql, {}));
  NotifyStep("create_schema", Status.STATUS_COMPLETED);

  NotifyStep("load_data", Status.STATUS_RUNNING);
  // Load data into tables using InsertValues
  console.log("Loading item table...");
  InsertValues(driver!, {
    count: ITEMS,
    tableName: "item",
    method: InsertMethod.PLAIN_QUERY,
    params: G.params({
      i_id: G.int32Seq(1, ITEMS),
      i_im_id: G.int32Seq(1, ITEMS),
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
      c_credit: G.str("GC"),
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

// ============================================================================
// NEWORD - New Order Transaction
// PG function: NEWORD(no_w_id, no_max_w_id, no_d_id, no_c_id, no_o_ol_cnt, no_d_next_o_id)
// ============================================================================
const newordWIdGen = NewGenByRule(0, G.int32(1, WAREHOUSES));
const newordDIdGen = NewGenByRule(1, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const newordCIdGen = NewGenByRule(2, G.int32(1, CUSTOMERS_PER_DISTRICT));
const newordOOlCntGen = NewGenByRule(3, G.int32(5, 15));
const newordDNextOIdGen = NewGenByRule(4, G.int32(1, 100000));
const newordItemIdGen = NewGenByRule(5, G.int32(1, ITEMS));
const newordQuantityGen = NewGenByRule(6, G.int32(1, 10));
const newordSQuantityGen = NewGenByRule(7, G.int32(10, 100));
const newordDistInfoGen = NewGenByRule(8, G.str(24, AB.enNum));
const newordAmountGen = NewGenByRule(9, G.double(1, 10000));

export function new_order() {
  // PG params: no_w_id, no_max_w_id, no_d_id, no_c_id, no_o_ol_cnt, no_d_next_o_id
  const no_w_id = newordWIdGen.next();
  const no_d_id = newordDIdGen.next();
  const no_c_id = newordCIdGen.next();
  const no_o_ol_cnt = newordOOlCntGen.next();
  const no_d_next_o_id = newordDNextOIdGen.next();
  const no_o_all_local = 1;

  // PG: SELECT c_discount, c_last, c_credit, w_tax
  //     FROM customer, warehouse
  //     WHERE warehouse.w_id = no_w_id AND customer.c_w_id = no_w_id
  //       AND customer.c_d_id = no_d_id AND customer.c_id = no_c_id

  // BUG: https://git.picodata.io/core/picodata/-/issues/2659
  // driver!.runQuery(getQuery("neword_get_customer_warehouse"), {
  //   w_id: no_w_id,
  //   d_id: no_d_id,
  //   c_id: no_c_id,
  // });

  // PG: UPDATE district SET d_next_o_id = d_next_o_id + 1
  //     WHERE d_id = no_d_id AND d_w_id = no_w_id
  //     RETURNING d_next_o_id - 1, d_tax INTO no_d_next_o_id, no_d_tax
  driver!.runQuery(getQuery("neword_get_district"), {
    d_id: no_d_id,
    w_id: no_w_id,
  });

  driver!.runQuery(getQuery("neword_update_district"), {
    d_id: no_d_id,
    w_id: no_w_id,
  });

  // PG: INSERT INTO ORDERS (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)
  //     VALUES (no_d_next_o_id, no_d_id, no_w_id, no_c_id, current_timestamp, no_o_ol_cnt, no_o_all_local)
  // BUG: https://git.picodata.io/core/picodata/-/issues/2659
  driver!.runQuery(getQuery("neword_insert_order"), {
    o_id: no_d_next_o_id,
    d_id: no_d_id,
    w_id: no_w_id,
    c_id: no_c_id,
    ol_cnt: no_o_ol_cnt,
    all_local: no_o_all_local,
  });

  // PG: INSERT INTO NEW_ORDER (no_o_id, no_d_id, no_w_id)
  //     VALUES (no_d_next_o_id, no_d_id, no_w_id)
  driver!.runQuery(getQuery("neword_insert_new_order"), {
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

    // PG: SELECT i_price, i_name, i_data FROM item WHERE i_id = item_id_array[loop_counter]
    driver!.runQuery(getQuery("neword_get_item"), {
      i_id: item_id,
    });

    // PG: SELECT s_quantity, s_data, s_dist_01..s_dist_10 FROM stock
    //     WHERE s_i_id = item_id AND s_w_id = no_w_id
    driver!.runQuery(getQuery("neword_get_stock"), {
      i_id: item_id,
      w_id: no_w_id,
    });

    // PG: UPDATE stock SET s_quantity = ..., s_ytd = s_ytd + quantity,
    //     s_order_cnt = s_order_cnt + 1, s_remote_cnt = s_remote_cnt + ...
    driver!.runQuery(getQuery("neword_update_stock"), {
      quantity: no_s_quantity,
      ol_quantity: quantity,
      remote_cnt: 0,
      i_id: item_id,
      w_id: no_w_id,
    });

    // BUG: https://git.picodata.io/core/picodata/-/issues/2659
    // PG: INSERT INTO order_line (ol_o_id, ol_d_id, ol_w_id, ol_number, ol_i_id,
    //     ol_supply_w_id, ol_quantity, ol_amount, ol_dist_info)
    // driver!.runQuery(getQuery("neword_insert_order_line"), {
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
// PG function: PAYMENT(p_w_id, p_d_id, p_c_w_id, p_c_d_id, p_c_id_in,
//                      byname, p_h_amount, p_c_last_in)
// ============================================================================
const paymentWIdGen = NewGenByRule(10, G.int32(1, WAREHOUSES));
const paymentDIdGen = NewGenByRule(11, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const paymentCWIdGen = NewGenByRule(12, G.int32(1, WAREHOUSES));
const paymentCDIdGen = NewGenByRule(13, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const paymentCIdGen = NewGenByRule(14, G.int32(1, CUSTOMERS_PER_DISTRICT));
const paymentHAmountGen = NewGenByRule(15, G.double(1, 5000));
const paymentCLastGen = NewGenByRule(16, G.strSeq(6, 16));
const paymentHDataGen = NewGenByRule(17, G.str(12, 24, AB.enSpc));

export function payments() {
  // PG params: p_w_id, p_d_id, p_c_w_id, p_c_d_id, p_c_id (= p_c_id_in),
  //            byname, p_h_amount, p_c_last (= p_c_last_in)
  const p_w_id = paymentWIdGen.next();
  const p_d_id = paymentDIdGen.next();
  const p_c_w_id = paymentCWIdGen.next();
  const p_c_d_id = paymentCDIdGen.next();
  const p_c_id = paymentCIdGen.next();
  const byname = 0;
  const p_h_amount = paymentHAmountGen.next();
  const p_c_last = paymentCLastGen.next();
  const h_data = paymentHDataGen.next();

  // PG: UPDATE warehouse SET w_ytd = w_ytd + p_h_amount
  //     WHERE w_id = p_w_id RETURNING w_name INTO p_w_name
  driver!.runQuery(getQuery("payment_update_warehouse"), {
    w_id: p_w_id,
    amount: p_h_amount,
  });

  // PG: (get w_name, split from RETURNING since picodata has no RETURNING)
  driver!.runQuery(getQuery("payment_get_warehouse"), {
    w_id: p_w_id,
  });

  // PG: UPDATE district SET d_ytd = d_ytd + p_h_amount
  //     WHERE d_w_id = p_w_id AND d_id = p_d_id RETURNING d_name INTO p_d_name
  driver!.runQuery(getQuery("payment_update_district"), {
    w_id: p_w_id,
    d_id: p_d_id,
    amount: p_h_amount,
  });

  // PG: (get d_name, split from RETURNING since picodata has no RETURNING)
  driver!.runQuery(getQuery("payment_get_district"), {
    w_id: p_w_id,
    d_id: p_d_id,
  });

  // PG: IF (byname = 1) THEN ... ELSE
  //     SELECT c_balance, c_credit INTO p_c_balance, p_c_credit
  //     FROM customer WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id
  driver!.runQuery(getQuery("payment_get_customer_by_id"), {
    w_id: p_c_w_id,
    d_id: p_c_d_id,
    c_id: p_c_id,
  });

  // PG: UPDATE customer SET c_balance = c_balance - p_h_amount,
  //     c_ytd_payment = c_ytd_payment + p_h_amount, c_payment_cnt = c_payment_cnt + 1
  //     WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id
  driver!.runQuery(getQuery("payment_update_customer"), {
    w_id: p_c_w_id,
    d_id: p_c_d_id,
    c_id: p_c_id,
    amount: p_h_amount,
  });

  // [INSERT - commented out]
  // PG: INSERT INTO history (h_c_d_id, h_c_w_id, h_c_id, h_d_id, h_w_id, h_date, h_amount, h_data)
  //     VALUES (p_c_d_id, p_c_w_id, p_c_id, p_d_id, p_w_id, current_timestamp, p_h_amount, h_data)
  driver!.runQuery(getQuery("payment_insert_history"), {
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
// PG function: OSTAT(os_w_id, os_d_id, os_c_id, byname, os_c_last)
// ============================================================================
const ostatWIdGen = NewGenByRule(20, G.int32(1, WAREHOUSES));
const ostatDIdGen = NewGenByRule(21, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const ostatCIdGen = NewGenByRule(22, G.int32(1, CUSTOMERS_PER_DISTRICT));
const ostatCLastGen = NewGenByRule(23, G.str(8, 16));
const ostatOIdGen = NewGenByRule(24, G.int32(1, 100000));

export function order_status() {
  // PG params: os_w_id, os_d_id, os_c_id, byname, os_c_last
  // PG declared: os_o_id
  const os_w_id = ostatWIdGen.next();
  const os_d_id = ostatDIdGen.next();
  const os_c_id = ostatCIdGen.next();
  const byname = 0;
  const os_c_last = ostatCLastGen.next();
  const os_o_id = ostatOIdGen.next();

  // PG: IF (byname = 1) THEN ... ELSE
  //     SELECT c_balance, c_first, c_middle, c_last
  //     INTO os_c_balance, os_c_first, os_c_middle, os_c_last
  //     FROM customer WHERE c_id = os_c_id AND c_d_id = os_d_id AND c_w_id = os_w_id
  driver!.runQuery(getQuery("ostat_get_customer_by_id"), {
    c_id: os_c_id,
    d_id: os_d_id,
    w_id: os_w_id,
  });

  // PG: SELECT o_id, o_carrier_id, o_entry_d
  //     INTO os_o_id, os_o_carrier_id, os_entdate
  //     FROM orders WHERE o_d_id = os_d_id AND o_w_id = os_w_id AND o_c_id = os_c_id
  //     ORDER BY o_id DESC LIMIT 1
  driver!.runQuery(getQuery("ostat_get_last_order"), {
    d_id: os_d_id,
    w_id: os_w_id,
    c_id: os_c_id,
  });

  // PG: SELECT ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_delivery_d
  //     FROM order_line WHERE ol_o_id = os_o_id
  driver!.runQuery(getQuery("ostat_get_order_lines"), {
    o_id: os_o_id,
    d_id: os_d_id,
    w_id: os_w_id,
  });
}

// ============================================================================
// DELIVERY - Delivery Transaction
// PG function: DELIVERY(d_w_id, d_o_carrier_id)
// ============================================================================
const deliveryWIdGen = NewGenByRule(25, G.int32(1, WAREHOUSES));
const deliveryOCarrierIdGen = NewGenByRule(26, G.int32(1, 10));
const deliveryDateGen = NewGenByRule(27, G.datetimeConst(new Date()));
const deliveryNoOIdGen = NewGenByRule(28, G.int32(1, 100000));
const deliveryCIdGen = NewGenByRule(29, G.int32(1, CUSTOMERS_PER_DISTRICT));
const deliveryOlTotalGen = NewGenByRule(30, G.double(1, 10000));

export function delivery() {
  // PG params: d_w_id, d_o_carrier_id
  const d_w_id = deliveryWIdGen.next();
  const d_o_carrier_id = deliveryOCarrierIdGen.next();
  const delivery_d = deliveryDateGen.next();

  // PG: d_id_in_array SMALLINT[] := ARRAY[1,2,3,4,5,6,7,8,9,10]
  // Process each district
  for (let d_id = 1; d_id <= DISTRICTS_PER_WAREHOUSE; d_id++) {
    // PG declared: o_id_array, c_id_array, sum_amounts (per-district in batch)
    const d_no_o_id = deliveryNoOIdGen.next();
    const d_c_id = deliveryCIdGen.next();
    const d_ol_total = deliveryOlTotalGen.next();

    // PG: select min(select_new_order.no_o_id)
    //     from new_order where no_d_id = d_ids and no_w_id = d_w_id
    driver!.runQuery(getQuery("delivery_get_min_new_order"), {
      d_id,
      w_id: d_w_id,
    });

    // PG: DELETE FROM new_order WHERE no_d_id = d_ids AND no_w_id = d_w_id
    //     AND no_o_id = (select min(...))
    driver!.runQuery(getQuery("delivery_delete_new_order"), {
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    // PG: (SELECT DISTINCT o_c_id FROM orders WHERE o_id = ol_o_id AND o_d_id = ol_d_id AND o_w_id = d_w_id)
    driver!.runQuery(getQuery("delivery_get_order"), {
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    // PG: UPDATE orders SET o_carrier_id = d_o_carrier_id
    //     WHERE orders.o_id = ids.o_id AND o_d_id = ids.d_id AND o_w_id = d_w_id
    driver!.runQuery(getQuery("delivery_update_order"), {
      carrier_id: d_o_carrier_id,
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    // PG: UPDATE order_line SET ol_delivery_d = current_timestamp
    //     WHERE ol_o_id = ids.o_id AND ol_d_id = ids.d_id AND ol_w_id = d_w_id
    driver!.runQuery(getQuery("delivery_update_order_line"), {
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    // PG: sum(ol_amount) AS sum_amount FROM order_line_update GROUP BY ol_d_id, ol_o_id
    driver!.runQuery(getQuery("delivery_get_order_line_amount"), {
      o_id: d_no_o_id,
      d_id,
      w_id: d_w_id,
    });

    // PG: UPDATE customer SET c_balance = COALESCE(c_balance,0) + ids_and_sums.sum_amounts,
    //     c_delivery_cnt = c_delivery_cnt + 1
    //     WHERE customer.c_id = ids_and_sums.c_id AND c_d_id = ids_and_sums.d_id AND c_w_id = d_w_id
    driver!.runQuery(getQuery("delivery_update_customer"), {
      amount: d_ol_total,
      c_id: d_c_id,
      d_id,
      w_id: d_w_id,
    });
  }
}

// ============================================================================
// SLEV - Stock Level Transaction
// PG function: SLEV(st_w_id, st_d_id, threshold)
// ============================================================================
const slevWIdGen = NewGenByRule(35, G.int32(1, WAREHOUSES));
const slevDIdGen = NewGenByRule(36, G.int32(1, DISTRICTS_PER_WAREHOUSE));
const slevThresholdGen = NewGenByRule(37, G.int32(10, 20));
const slevNextOIdGen = NewGenByRule(38, G.int32(3001, 100000));

export function stock_level() {
  // PG params: st_w_id, st_d_id, threshold
  // PG declared: stock_count
  const st_w_id = slevWIdGen.next();
  const st_d_id = slevDIdGen.next();
  const threshold = slevThresholdGen.next();
  const st_next_o_id = slevNextOIdGen.next();

  // PG: (implicit) SELECT d_next_o_id FROM district WHERE d_w_id = st_w_id AND d_id = st_d_id
  driver!.runQuery(getQuery("slev_get_district"), {
    w_id: st_w_id,
    d_id: st_d_id,
  });

  // PG: SELECT COUNT(DISTINCT (s_i_id)) INTO stock_count
  //     FROM order_line, stock, district
  //     WHERE ol_w_id = st_w_id AND ol_d_id = st_d_id
  //       AND d_w_id = st_w_id AND d_id = st_d_id
  //       AND (ol_o_id < d_next_o_id) AND ol_o_id >= (d_next_o_id - 20)
  //       AND s_w_id = st_w_id AND s_i_id = ol_i_id AND s_quantity < threshold
  driver!.runQuery(getQuery("slev_stock_count"), {
    w_id: st_w_id,
    d_id: st_d_id,
    next_o_id: st_next_o_id,
    min_o_id: st_next_o_id - 20,
    threshold,
  });
}

export function teardown() {
  NotifyStep("workload", Status.STATUS_COMPLETED);
  Teardown();
}
