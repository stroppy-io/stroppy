package config

import (
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/stroppy-io/stroppy/internal/static"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

func NewExampleConfig() *stroppy.Config { //nolint: funlen,maintidx // allow in example
	return &stroppy.Config{
		Version: stroppy.Version,
		Run: &stroppy.RunConfig{
			Logger: &stroppy.LoggerConfig{
				LogLevel: stroppy.LoggerConfig_LOG_LEVEL_INFO,
				LogMode:  stroppy.LoggerConfig_LOG_MODE_PRODUCTION,
			},
			RunId: uuid.New().String(),
			Seed:  uint64(time.Now().Unix()), //nolint: gosec // allow
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
						{Key: "max_conns", Type: &stroppy.Value_Int32{Int32: 100}},     //nolint: mnd // not need const value here
						{Key: "min_conns", Type: &stroppy.Value_Int32{Int32: 1}},       //nolint: mnd // not need const value here
						{Key: "min_idle_conns", Type: &stroppy.Value_Int32{Int32: 10}}, //nolint: mnd // not need const value here
					},
				},
			},
			GoExecutor: &stroppy.GoExecutor{
				GoMaxProc:     toPtr[uint64](100), //nolint: mnd // not need const value here
				CancelOnError: toPtr[bool](true),
			},
			K6Executor: &stroppy.K6Executor{
				K6BinaryPath:   "./" + static.K6PluginFileName.String(),
				K6ScriptPath:   "./" + static.K6BenchmarkFileName.String(),
				K6SetupTimeout: durationpb.New(8400 * time.Second), //nolint: mnd // not need const value here
				K6Vus:          toPtr(uint64(10)),                  //nolint: mnd // not need const value here
				K6MaxVus:       toPtr(uint64(100)),                 //nolint: mnd // not need const value here
				K6Rate:         toPtr(uint64(1000)),                //nolint: mnd // not need const value here
				K6Duration:     durationpb.New(60 * time.Second),   //nolint: mnd // not need const value here
				OtlpExport: &stroppy.OtlpExport{
					OtlpGrpcEndpoint:  toPtr("localhost:4317"),
					OtlpMetricsPrefix: toPtr("k6_"),
				},
			},
			Steps: []*stroppy.RequestedStep{
				{
					Name:     "create_table",
					Executor: toPtr(stroppy.RequestedStep_EXECUTOR_TYPE_GO),
				},
				{
					Name:     "insert_data",
					Executor: toPtr(stroppy.RequestedStep_EXECUTOR_TYPE_GO),
				},
				{
					Name:     "warm_up",
					Executor: toPtr(stroppy.RequestedStep_EXECUTOR_TYPE_GO),
				},
				{
					Name:     "select_data",
					Executor: toPtr(stroppy.RequestedStep_EXECUTOR_TYPE_K6),
				},
			},
		},
		Benchmark: &stroppy.BenchmarkDescriptor{
			Name: "example",
			Steps: []*stroppy.StepDescriptor{
				{
					Name: "create_table",
					Units: []*stroppy.StepUnitDescriptor{
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name: "users",
										Columns: []*stroppy.ColumnDescriptor{
											{
												Name:       "id",
												SqlType:    "INT",
												PrimaryKey: true,
											},
											{
												Name:     "name",
												SqlType:  "TEXT",
												Nullable: true,
											},
											{
												Name:       "email",
												SqlType:    "TEXT",
												Constraint: "NOT NULL",
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Name:  "insert_data",
					Async: true,
					Units: []*stroppy.StepUnitDescriptor{
						{
							Count: 100000, //nolint: mnd // not need const value here
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
														Screw: 1.1, //nolint: mnd // not need const value here
													},
													Type: &stroppy.Generation_Rule_Int32Rules{
														Int32Rules: &stroppy.Generation_Rules_Int32Rule{
															Range: &stroppy.Generation_Range_Int32Range{
																Min: 1,      //nolint: mnd // not need const value here
																Max: 100000, //nolint: mnd // not need const value here
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
																Min: 1,   //nolint: mnd // not need const value here
																Max: 100, //nolint: mnd // not need const value here
															},
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32Range{
																	{
																		Min: 65, //nolint: mnd // not need const value here
																		Max: 90, //nolint: mnd // not need const value here
																	},
																	{
																		Min: 97,  //nolint: mnd // not need const value here
																		Max: 122, //nolint: mnd // not need const value here
																	},
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
																Min: 10, //nolint: mnd // not need const value here
																Max: 50, //nolint: mnd // not need const value here
															},
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32Range{
																	{
																		Min: 65, //nolint: mnd // not need const value here
																		Max: 90, //nolint: mnd // not need const value here
																	},
																	{
																		Min: 97,  //nolint: mnd // not need const value here
																		Max: 122, //nolint: mnd // not need const value here
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
					Name:  "warm_up",
					Async: true,
					Units: []*stroppy.StepUnitDescriptor{
						{
							Count: 1000, //nolint: mnd // not need const value here
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
																Min: 1,   //nolint: mnd // not need const value here
																Max: 100, //nolint: mnd // not need const value here
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
					Name:  "select_data",
					Async: true,
					Units: []*stroppy.StepUnitDescriptor{
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
																Min: 1,   //nolint: mnd // not need const value here
																Max: 100, //nolint: mnd // not need const value here
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
					Units: []*stroppy.StepUnitDescriptor{
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
																		Min: 1,   //nolint: mnd // not need const value here
																		Max: 100, //nolint: mnd // not need const value here
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
																		Min: 1,   //nolint: mnd // not need const value here
																		Max: 100, //nolint: mnd // not need const value here
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
			},
		},
	}
}
