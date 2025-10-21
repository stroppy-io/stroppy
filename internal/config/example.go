package config

import (
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/durationpb"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

func ptr[T any](x T) *T {
	return &x
}

//nolint:mnd // it is a huge magic config by itself
func NewExampleConfig() *stroppy.ConfigFile { //nolint: funlen,maintidx,mnd // allow in example
	return &stroppy.ConfigFile{
		Global: &stroppy.GlobalConfig{
			Version: stroppy.Version,
			RunId:   uuid.New().String(),
			Seed:    uint64(time.Now().Unix()), //nolint: gosec // allow
			Metadata: map[string]string{
				"example": "stroppy_metadata",
			},
			Driver: &stroppy.DriverConfig{
				DriverType: stroppy.DriverConfig_DRIVER_TYPE_POSTGRES,
				Url:        "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable",
				DbSpecific: &stroppy.Value_Struct{
					Fields: []*stroppy.Value{
						{Key: "trace_log_level", Type: &stroppy.Value_String_{String_: "warn"}},
						{Key: "max_conn_lifetime", Type: &stroppy.Value_String_{String_: "1m"}},
						{Key: "max_conn_idle_time", Type: &stroppy.Value_String_{String_: "30s"}},
						{Key: "max_conns", Type: &stroppy.Value_Int32{Int32: 100}},
						{Key: "min_conns", Type: &stroppy.Value_Int32{Int32: 1}},
						{Key: "min_idle_conns", Type: &stroppy.Value_Int32{Int32: 10}},
					},
				},
			},
			Logger: &stroppy.LoggerConfig{
				LogLevel: stroppy.LoggerConfig_LOG_LEVEL_INFO,
				LogMode:  stroppy.LoggerConfig_LOG_MODE_PRODUCTION,
			},
		},
		Executors: []*stroppy.ExecutorConfig{
			{
				Name: "single-vus-single-step",
				K6: &stroppy.K6Options{
					Scenario: &stroppy.K6Scenario{
						Executor: &stroppy.K6Scenario_PerVuIterations{
							PerVuIterations: &stroppy.PerVuIterations{
								Vus:        1,
								Iterations: 1,
							},
						},
					},
				},
			},
			{
				Name: "max-workload",
				K6: &stroppy.K6Options{
					Scenario: &stroppy.K6Scenario{
						Executor: &stroppy.K6Scenario_ConstantArrivalRate{
							ConstantArrivalRate: &stroppy.ConstantArrivalRate{
								Rate: 1000,
								TimeUnit: &durationpb.Duration{
									Seconds: 1,
								},
								PreAllocatedVus: 1,
								MaxVus:          100,
							},
						},
					},
				},
			},
		},
		Exporters: []*stroppy.ExporterConfig{
			{
				Name: "otlp-cloud-export",
				OtlpExport: &stroppy.OtlpExport{
					OtlpGrpcEndpoint:  toPtr("localhost:4317"),
					OtlpMetricsPrefix: toPtr("k6_"),
				},
			},
		},
		Steps: []*stroppy.Step{
			{
				Name:     "create_table",
				Workload: "create_table",
				Executor: "single-vus-single-step",
				Exporter: ptr("otlp-cloud-export"),
			},
			{
				Name:     "insert_data",
				Workload: "insert_data",
				Executor: "max-workload",
				Exporter: ptr("otlp-cloud-export"),
			},
			{
				Name:     "warm_up",
				Workload: "warm_up",
				Executor: "max-workload",
				Exporter: ptr("otlp-cloud-export"),
			},
			{
				Name:     "select_data",
				Workload: "select_data",
				Executor: "max-workload",
				Exporter: ptr("otlp-cloud-export"),
			},
			{
				Name:     "transaction",
				Workload: "transaction",
				Executor: "max-workload",
				Exporter: ptr("otlp-cloud-export"),
			},
			{
				Name:     "clean_up",
				Workload: "clean_up",
				Executor: "single-vus-single-step",
				Exporter: ptr("otlp-cloud-export"),
			},
		},
		Benchmark: &stroppy.BenchmarkDescriptor{
			Name: "example",
			Workloads: []*stroppy.WorkloadDescriptor{
				{
					Name: "create_table",
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name: "users",
										Columns: []*stroppy.ColumnDescriptor{
											{Name: "id", SqlType: "INT", PrimaryKey: true},
											{Name: "name", SqlType: "TEXT", Nullable: true},
											{
												Name:       "email",
												SqlType:    "TEXT",
												Constraint: "NOT NULL",
											},
										},
									},
								},
							},
							Count: 1,
						},
					},
				},
				{
					Name: "insert_data",
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Count: 100000,
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "insert",
										//noinspection *
										Sql: "INSERT INTO users (id, name, email) VALUES (${id}, ${name}, ${email})",
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "id",
												GenerationRule: &stroppy.Generation_Rule{
													Unique: toPtr(true),
													Distribution: &stroppy.Generation_Distribution{
														Type:  stroppy.Generation_Distribution_ZIPF,
														Screw: 1.1,
													},
													Type: &stroppy.Generation_Rule_Int32Rules{
														Int32Rules: &stroppy.Generation_Rules_Int32Rule{
															Range: &stroppy.Generation_Range_Int32Range{
																Min: 1,
																Max: 100000,
															},
														},
													},
												},
											},
											{
												Name: "name",
												GenerationRule: &stroppy.Generation_Rule{
													Type: &stroppy.Generation_Rule_StringRules{
														StringRules: &stroppy.Generation_Rules_StringRule{
															LenRange: &stroppy.Generation_Range_UInt64Range{
																Min: 1,
																Max: 100,
															},
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32Range{
																	{Min: 65, Max: 90},
																	{Min: 97, Max: 122},
																},
															},
														},
													},
												},
											},
											{
												Name: "email",
												GenerationRule: &stroppy.Generation_Rule{
													Type: &stroppy.Generation_Rule_StringRules{
														StringRules: &stroppy.Generation_Rules_StringRule{
															LenRange: &stroppy.Generation_Range_UInt64Range{
																Min: 10,
																Max: 50,
															},
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32Range{
																	{Min: 65, Max: 90},
																	{Min: 97, Max: 122},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Name: "warm_up",
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Count: 1000,
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "select",
										//noinspection *
										Sql: "SELECT * FROM users WHERE id = ${id}",
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "id",
												GenerationRule: &stroppy.Generation_Rule{
													Type: &stroppy.Generation_Rule_Int32Rules{
														Int32Rules: &stroppy.Generation_Rules_Int32Rule{
															Range: &stroppy.Generation_Range_Int32Range{
																Min: 1,
																Max: 100,
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Name: "select_data",
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Count: 1,
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "select",
										//noinspection *
										Sql: "SELECT * FROM users WHERE id = ${id}",
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "id",
												GenerationRule: &stroppy.Generation_Rule{
													Type: &stroppy.Generation_Rule_Int32Rules{
														Int32Rules: &stroppy.Generation_Rules_Int32Rule{
															Range: &stroppy.Generation_Range_Int32Range{
																Min: 1,
																Max: 100,
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Name: "transaction",
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Count: 1,
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Transaction{
									Transaction: &stroppy.TransactionDescriptor{
										Name:           "transaction",
										IsolationLevel: stroppy.TxIsolationLevel_TX_ISOLATION_LEVEL_REPEATABLE_READ,
										Queries: []*stroppy.QueryDescriptor{
											{
												Name: "select",
												//noinspection *
												Sql: "SELECT * FROM users WHERE id = ${id}",
												Params: []*stroppy.QueryParamDescriptor{
													{
														Name: "id",
														GenerationRule: &stroppy.Generation_Rule{
															Type: &stroppy.Generation_Rule_Int32Rules{
																Int32Rules: &stroppy.Generation_Rules_Int32Rule{
																	Range: &stroppy.Generation_Range_Int32Range{
																		Min: 1,
																		Max: 100,
																	},
																},
															},
														},
													},
												},
											},
											{
												Name: "select2",
												//noinspection *
												Sql: "SELECT * FROM users WHERE id = ${id}",
												Params: []*stroppy.QueryParamDescriptor{
													{
														Name: "id",
														GenerationRule: &stroppy.Generation_Rule{
															Type: &stroppy.Generation_Rule_Int32Rules{
																Int32Rules: &stroppy.Generation_Rules_Int32Rule{
																	Range: &stroppy.Generation_Range_Int32Range{
																		Min: 1,
																		Max: 100,
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Name: "clean_up",
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Count: 1,
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "drop users",
										Sql:  "DROP TABLE users",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
