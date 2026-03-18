import { Options } from "k6/options";
import { Teardown } from "k6/x/stroppy";

import { DriverX, ENV } from "./helpers.ts";
import { parse_sql } from "./parse_sql.js";

const SQL_FILE = ENV("SQL_FILE", "", "Path to SQL file (automatically set if .sql file provided as argument)");

export const options: Options = {};

const driver = DriverX.create().setup({
  url: ENV("DRIVER_URL", "postgres://postgres:postgres@localhost:5432", "Database connection URL"),
  driverType: "postgres",
});

const parsedQueries = parse_sql(open(SQL_FILE));

export default function () {
  parsedQueries().forEach((query) => {
    driver.exec(query, {});
  });
}

export function teardown() {
  Teardown();
}
