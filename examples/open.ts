import { Options } from "k6/options";

export const options: Options = {
  setupTimeout: "5m",
  scenarios: {
    test_func: {
      executor: "shared-iterations",
      exec: "test_func",
      vus: 1,
      iterations: 1,
    },
  },
};

const test = open("tpcb.sql");

export function test_func(): void {
  console.log(test);
}
