import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";

import { DriverX, ENV, declareDriverSetup } from "./helpers.ts";
import { parse_sql } from "./parse_sql.js";

const SQL_FILE = ENV("SQL_FILE", "", "Path to SQL file (automatically set if .sql file provided as argument)");

export const options: Options = {};

const driverConfig = declareDriverSetup(0, {
  url: "postgres://postgres:postgres@localhost:5432",
  driverType: "postgres",
});

const driver = DriverX.create().setup(driverConfig);

const parsedQueries = parse_sql(open(SQL_FILE));

export default function () {
  parsedQueries().forEach((query) => {
    driver.exec(query, {});
  });
}

export function teardown() {
  Teardown();
}
