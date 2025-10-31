import { ConfigFile } from './stroppy.pb';
const config: ConfigFile = {
  exporters: [
    {
      name: "tpcc-metrics",
      otlpExport: {
        otlpGrpcEndpoint: "localhost:4317",
        otlpEndpointInsecure: false,
        otlpMetricsPrefix: "stroppy_k6_tpcc_",
      },
    }
  ],
  executors: [
    {
      name: "single-execution",
      k6: {
        k6Args: [],
        scenario: {
          executor: {},
        },
      },
    },
    {
      name: "data-load-executor",
      k6: {
        k6Args: [],
        scenario: {
          executor: {},
        },
      },
    },
    {
      name: "tpcc-benchmark",
      k6: {
        k6Args: [],
        scenario: {
          executor: {},
        },
      },
    },
    {
      name: "tpcc-benchmark-v3",
      k6: {
        k6Args: [],
        scenario: {
          executor: {},
        },
      },
    }
  ],
  steps: [
    {
      name: "create_schema",
      workload: "create_schema",
      executor: "single-execution",
      exporter: "tpcc-metrics",
    },
    {
      name: "create_procedures",
      workload: "create_stored_procedures",
      executor: "single-execution",
      exporter: "tpcc-metrics",
    },
    {
      name: "load_data",
      workload: "load_data",
      executor: "data-load-executor",
      exporter: "tpcc-metrics",
    },
    {
      name: "tpcc_workload",
      workload: "tpcc_workload",
      executor: "tpcc-benchmark",
      exporter: "tpcc-metrics",
    },
    {
      name: "cleanup",
      workload: "cleanup",
      executor: "single-execution",
      exporter: "tpcc-metrics",
    }
  ],
  sideCars: [],
  global: {
    version: "v1.0.2",
    runId: "4e4ba39c-4c85-4105-980c-589acc902d25",
    seed: "987654321",
    metadata: {
      approach: "stored_procedures",
      benchmark_type: "tpc_c_with_procedures",
      description: "TPC-C Benchmark with Stored Procedures",
      specification_version: "5.11",
      warehouses: "10",
    },
    driver: {
      url: "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable",
      driverType: "DRIVER_TYPE_POSTGRES",
      dbSpecific: {
        fields: [
          {
            type: {},
            key: "trace_log_level",
          },
          {
            type: {},
            key: "max_conn_lifetime",
          },
          {
            type: {},
            key: "max_conn_idle_time",
          },
          {
            type: {},
            key: "max_conns",
          },
          {
            type: {},
            key: "min_conns",
          },
          {
            type: {},
            key: "min_idle_conns",
          }
        ],
      },
    },
    logger: {
      logLevel: "LOG_LEVEL_INFO",
      logMode: "LOG_MODE_PRODUCTION",
    },
  },
  benchmark: {
    name: "tpcc_postgresq",
    workloads: [
      {
        name: "create_schema",
        units: [
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          }
        ],
      },
      {
        name: "create_stored_procedures",
        units: [
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          }
        ],
      },
      {
        name: "load_data",
        units: [
          {
            count: "100000",
            descriptor: {
              type: {},
            },
          },
          {
            count: "10",
            descriptor: {
              type: {},
            },
          },
          {
            count: "100",
            descriptor: {
              type: {},
            },
          },
          {
            count: "300000",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1000000",
            descriptor: {
              type: {},
            },
          }
        ],
      },
      {
        name: "tpcc_workload",
        units: [
          {
            count: "45",
            descriptor: {
              type: {},
            },
          },
          {
            count: "43",
            descriptor: {
              type: {},
            },
          },
          {
            count: "4",
            descriptor: {
              type: {},
            },
          },
          {
            count: "4",
            descriptor: {
              type: {},
            },
          },
          {
            count: "4",
            descriptor: {
              type: {},
            },
          }
        ],
        async: true,
      },
      {
        name: "cleanup",
        units: [
          {
            count: "1",
            descriptor: {
              type: {},
            },
          },
          {
            count: "1",
            descriptor: {
              type: {},
            },
          }
        ],
      }
    ],
  },
};
