import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";
import { DriverX, Step, ENV, declareDriverSetup } from "./helpers.ts";
import { parse_sql } from "./parse_sql.js";

const SQL_FILE = ENV("SQL_FILE", "", "Path to SQL file (automatically set if .sql file provided as argument)");

export const options: Options = {
  vus: 1,
  iterations: 1,
};

const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
});

const driver = DriverX.create().setup(driverConfig);

const queries = parse_sql(open(SQL_FILE));

export function setup() {
  Step.begin("workload");
  return;
}

export default function (): void {
  queries().forEach((query) => {
    console.log(`tpc-ds-like: ${query.name}`);
    driver.exec(query, {});
  });
}

export function teardown() {
  Step.end("workload");
  Teardown();
}
