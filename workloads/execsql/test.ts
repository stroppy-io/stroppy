import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverConfig_DriverType } from "./stroppy.pb.js";
import { DriverX, Step } from "./helpers.ts";
import { parse_sql } from "./parse_sql.js";

export const options: Options = {};

const driver = DriverX.fromConfig({
  driver: {
    url: __ENV.DRIVER_URL || "postgres://postgres:postgres@localhost:5432",
    driverType: DriverConfig_DriverType.DRIVER_TYPE_POSTGRES,
  },
});

const queries = parse_sql(open(__ENV.SQL_FILE));

export function setup (): void {
  Step.begin("workload");
}

export default function (): void {
  queries().forEach((query) => {
    driver.runQuery(query, {});
  });
}

export function teardown (): void {
  Step.end("workload");
  Teardown();
}
