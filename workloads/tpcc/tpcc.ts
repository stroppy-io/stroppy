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

import { parse_sql_with_groups } from "./parse_sql_2.js";

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

const sections = parse_sql_with_groups(open(__SQL_FILE));

export function setup() {
  NotifyStep("create_schema", Status.STATUS_RUNNING);
  sections["drop_schema"].forEach((query) => driver.runQuery(query.sql, {}));
  sections["create_schema"].forEach((query) => driver.runQuery(query.sql, {}));
  NotifyStep("create_schema", Status.STATUS_COMPLETED);

  NotifyStep("load_data", Status.STATUS_RUNNING);
  // Load data into tables using InsertValues with COPY_FROM method
  console.log("Loading items...");
  {
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
  }
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
  driver.runQuery("SELECT NEWORD(:w_id, :max_w_id, :d_id, :c_id, :ol_cnt, 0)", {
    w_id: newOrderWarehouseGen.next(),
    max_w_id: newOrderMaxWarehouseGen.next(),
    d_id: newOrderDistrictGen.next(),
    c_id: newOrderCustomerGen.next(),
    ol_cnt: newOrderOlCntGen.next(),
  });
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
    kind: {
      oneofKind: "stringRange",
      stringRange: { maxLen: "16", minLen: "6" },
    },
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
    kind: {
      oneofKind: "stringRange",
      stringRange: { maxLen: "16", minLen: "8" },
    },
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
