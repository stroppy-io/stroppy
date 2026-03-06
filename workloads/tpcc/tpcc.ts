import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverConfig_DriverType } from "./stroppy.pb.js";
import { AB, C, R, Step, DriverX, S } from "./helpers.ts";
import { parse_sql_with_sections } from "./parse_sql.js";

const DURATION = __ENV.DURATION || "1h";
const VUS_SCALE = +(__ENV.VUS_SCALE || 1);
export const options: Options = {
  setupTimeout: "5m",
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
// TPCC Configuration Constants
const POOL_SIZE = +(__ENV.POOL_SIZE || 100);
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
    connectionType: { is: {oneofKind:"sharedPool", sharedPool: {sharedConnections: POOL_SIZE}}},
    dbSpecific: {
      fields: [],
    },
  },
});

const sql = parse_sql_with_sections(open(__ENV.SQL_FILE));

export function setup() {
  Step("create_schema", () => {
    sql("drop_schema").forEach((query) => driver.runQuery(query, {}));
    sql("create_schema").forEach((query) => driver.runQuery(query, {}));
  });

  Step("load_data", () => {
    // Load data into tables using InsertValues with COPY_FROM method
    driver.insert("item", ITEMS, {
      method: "copy_from",
      params: {
        i_id: S.int32(1, ITEMS),
        i_im_id: S.int32(1, ITEMS), // WHY: not unique originaly
        i_name: R.str(14, 24, AB.enSpc),
        i_price: R.float(1, 100),
        i_data: R.str(26, 50, AB.enSpc),
      },
    });

    driver.insert("warehouse", WAREHOUSES, {
      method: "copy_from",
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
      method: "copy_from",
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
      method: "copy_from",
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
        c_credit: C.str("GC"), // TODO: "GC" | "BC" (good/bad credit), and 10% should be "BC"
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
      method: "copy_from",
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
  return;
}

const newOrderWarehouseGen = R.int32(1, WAREHOUSES).gen();
const newOrderMaxWarehouseGen = C.int32(WAREHOUSES).gen();
const newOrderDistrictGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const newOrderCustomerGen = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const newOrderOlCntGen = R.int32(5, 15).gen();

export function new_order() {
  driver.runQuery(sql("workload", "new_order")!, {
    w_id: newOrderWarehouseGen.next(),
    max_w_id: newOrderMaxWarehouseGen.next(),
    d_id: newOrderDistrictGen.next(),
    c_id: newOrderCustomerGen.next(),
    ol_cnt: newOrderOlCntGen.next(),
  });
}

const paymentWarehouseGen = R.int32(1, WAREHOUSES).gen();
const paymentDistrictGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCustomerWarehouseGen = R.int32(1, WAREHOUSES).gen();
const paymentCustomerDistrictGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const paymentCustomerGen = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const paymentAmountGen = R.double(1, 5000).gen();
const paymentCustomerLastGen = S.str(6, 16).gen();

export function payments() {
  driver.runQuery(sql("workload", "payment")!, {
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

const orderStatusWarehouseGen = R.int32(1, WAREHOUSES).gen();
const orderStatusDistrictGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const orderStatusCustomerGen = R.int32(1, CUSTOMERS_PER_DISTRICT).gen();
const orderStatusCustomerLastGen = R.str(8, 16).gen();

export function order_status() {
  driver.runQuery(sql("workload", "order_status")!, {
      os_w_id: orderStatusWarehouseGen.next(),
      os_d_id: orderStatusDistrictGen.next(),
      os_c_id: orderStatusCustomerGen.next(),
      byname: 0,
      os_c_last: orderStatusCustomerLastGen.next(),
    },
  );
}

const deliveryWarehouseGen = R.int32(1, WAREHOUSES).gen();
const deliveryCarrierGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();

export function delivery() {
  driver.runQuery(sql("workload", "delivery")!, {
    d_w_id: deliveryWarehouseGen.next(),
    d_o_carrier_id: deliveryCarrierGen.next(),
  });
}

const stockLevelWarehouseGen = R.int32(1, WAREHOUSES).gen();
const stockLevelDistrictGen = R.int32(1, DISTRICTS_PER_WAREHOUSE).gen();
const stockLevelThresholdGen = R.int32(10, 20).gen();

export function stock_level() {
  driver.runQuery(sql("workload", "stock_level")!, {
    st_w_id: stockLevelWarehouseGen.next(),
    st_d_id: stockLevelDistrictGen.next(),
    threshold: stockLevelThresholdGen.next(),
  });
}

export function teardown() {
  Step.begin("workload");
  Teardown();
}
