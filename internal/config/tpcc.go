package config

import (
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

//nolint:mnd,funlen,lll,maintidx,dupl,gosec // a huge and long magic constant
func NewTPCCConfig(warehouseMax int32) *stroppy.ConfigFile {
	itemsMax := int32(100000)

	return &stroppy.ConfigFile{
		Global: &stroppy.GlobalConfig{
			Version: "v1.0.2",
			RunId:   uuid.New().String(),
			Seed:    uint64(time.Now().Unix()), //nolint: gosec // allow
			Metadata: map[string]string{
				"benchmark_type":        "tpc_c_with_procedures",
				"description":           "TPC-C Benchmark with Stored Procedures",
				"specification_version": "5.11",
				"warehouses":            "10",
				"approach":              "stored_procedures",
			},
			Driver: &stroppy.DriverConfig{
				Url: "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable",
				DbSpecific: &stroppy.Value_Struct{
					Fields: []*stroppy.Value{
						{
							Type: &stroppy.Value_String_{
								String_: "warn",
							},
							Key: "trace_log_level",
						},
						{
							Type: &stroppy.Value_String_{
								String_: "5m",
							},
							Key: "max_conn_lifetime",
						},
						{
							Type: &stroppy.Value_String_{
								String_: "2m",
							},
							Key: "max_conn_idle_time",
						},
						{
							Type: &stroppy.Value_Int32{
								Int32: 300,
							},
							Key: "max_conns",
						},
						{
							Type: &stroppy.Value_Int32{
								Int32: 50,
							},
							Key: "min_conns",
						},
						{
							Type: &stroppy.Value_Int32{
								Int32: 100,
							},
							Key: "min_idle_conns",
						},
					},
				},
				DriverType: stroppy.DriverConfig_DriverType(1),
			},
			Logger: &stroppy.LoggerConfig{
				LogLevel: stroppy.LoggerConfig_LogLevel(1),
				LogMode:  stroppy.LoggerConfig_LogMode(1),
			},
		},
		Exporters: []*stroppy.ExporterConfig{{
			Name: "tpcc-metrics",
			OtlpExport: &stroppy.OtlpExport{
				OtlpGrpcEndpoint:     ptr("localhost:4317"),
				OtlpEndpointInsecure: ptr(false),
				OtlpMetricsPrefix:    ptr("stroppy_k6_tpcc_"),
			},
		}},
		Executors: []*stroppy.ExecutorConfig{
			{
				Name: "single-execution",
				K6: &stroppy.K6Options{
					Scenario: &stroppy.K6Scenario{
						MaxDuration: durationpb.New(time.Hour),
						Executor: &stroppy.K6Scenario_PerVuIterations{
							PerVuIterations: &stroppy.PerVuIterations{
								Vus:        1,
								Iterations: -1,
							},
						},
					},
				},
			},
			{
				Name: "data-load-executor",
				K6: &stroppy.K6Options{
					Scenario: &stroppy.K6Scenario{
						MaxDuration: durationpb.New(time.Hour),
						Executor: &stroppy.K6Scenario_PerVuIterations{
							PerVuIterations: &stroppy.PerVuIterations{
								Vus:        1,
								Iterations: -1,
							},
						},
					},
				},
			},
			{
				Name: "tpcc-benchmark",
				K6: &stroppy.K6Options{
					Scenario: &stroppy.K6Scenario{
						MaxDuration: durationpb.New(time.Hour),
						Executor: &stroppy.K6Scenario_ConstantArrivalRate{
							ConstantArrivalRate: &stroppy.ConstantArrivalRate{
								Rate: 500,
								TimeUnit: &durationpb.Duration{
									Seconds: 1,
								},
								Duration: &durationpb.Duration{
									Seconds: 1800,
								},
								PreAllocatedVus: 100,
								MaxVus:          2000,
							},
						},
					},
				},
			},
			{
				Name: "tpcc-benchmark-ramping-rate",
				K6: &stroppy.K6Options{
					Scenario: &stroppy.K6Scenario{
						MaxDuration: durationpb.New(time.Hour),
						Executor: &stroppy.K6Scenario_RampingArrivalRate{
							RampingArrivalRate: &stroppy.RampingArrivalRate{
								StartRate: 500,
								TimeUnit: &durationpb.Duration{
									Seconds: 1,
								},
								Stages: []*stroppy.RampingArrivalRate_RateStage{
									{
										Target: 500,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 1000,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 1500,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 2000,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 2500,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 3000,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 3500,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 4000,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 4500,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 5000,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 5500,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 6000,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
									{
										Target: 6500,
										Duration: &durationpb.Duration{
											Seconds: 600,
										},
									},
								},
								PreAllocatedVus: 100,
								MaxVus:          2000,
							},
						},
					},
				},
			},
		},
		Steps: []*stroppy.Step{
			{
				Name:     "create_schema",
				Workload: "create_schema",
				Executor: "single-execution",
				Exporter: ptr("tpcc-metrics"),
			},
			{
				Name:     "create_procedures",
				Workload: "create_stored_procedures",
				Executor: "single-execution",
				Exporter: ptr("tpcc-metrics"),
			},
			{
				Name:     "load_data",
				Workload: "load_data",
				Executor: "data-load-executor",
				Exporter: ptr("tpcc-metrics"),
			},
			{
				Name:     "tpcc_workload",
				Workload: "tpcc_workload",
				Executor: "tpcc-benchmark",
				Exporter: ptr("tpcc-metrics"),
			},
			{
				Name:     "cleanup",
				Workload: "cleanup",
				Executor: "single-execution",
				Exporter: ptr("tpcc-metrics"),
			},
		},
		Benchmark: &stroppy.BenchmarkDescriptor{
			Name: "tpcc_postgresql",
			Workloads: []*stroppy.WorkloadDescriptor{
				{
					Name: "create_schema",
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name:       "warehouse",
										DbSpecific: &stroppy.Value_Struct{},
										Columns: []*stroppy.ColumnDescriptor{
											{
												Name:       "w_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{Name: "w_name", SqlType: "VARCHAR(10)"},
											{Name: "w_street_1", SqlType: "VARCHAR(20)"},
											{Name: "w_street_2", SqlType: "VARCHAR(20)"},
											{Name: "w_city", SqlType: "VARCHAR(20)"},
											{Name: "w_state", SqlType: "CHAR(2)"},
											{Name: "w_zip", SqlType: "CHAR(9)"},
											{Name: "w_tax", SqlType: "DECIMAL(4,4)"},
											{Name: "w_ytd", SqlType: "DECIMAL(12,2)"},
										},
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name:       "district",
										DbSpecific: &stroppy.Value_Struct{},
										Columns: []*stroppy.ColumnDescriptor{
											{
												Name:       "d_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{
												Name:       "d_w_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
												Constraint: ptr("REFERENCES warehouse(w_id)"),
											},
											{Name: "d_name", SqlType: "VARCHAR(10)"},
											{Name: "d_street_1", SqlType: "VARCHAR(20)"},
											{Name: "d_street_2", SqlType: "VARCHAR(20)"},
											{Name: "d_city", SqlType: "VARCHAR(20)"},
											{Name: "d_state", SqlType: "CHAR(2)"},
											{Name: "d_zip", SqlType: "CHAR(9)"},
											{Name: "d_tax", SqlType: "DECIMAL(4,4)"},
											{Name: "d_ytd", SqlType: "DECIMAL(12,2)"},
											{Name: "d_next_o_id", SqlType: "INTEGER"},
										},
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name:       "customer",
										DbSpecific: &stroppy.Value_Struct{},
										Columns: []*stroppy.ColumnDescriptor{
											{
												Name:       "c_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{
												Name:       "c_d_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{
												Name:       "c_w_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
												Constraint: ptr("REFERENCES warehouse(w_id)"),
											},
											{Name: "c_first", SqlType: "VARCHAR(16)"},
											{Name: "c_middle", SqlType: "CHAR(2)"},
											{Name: "c_last", SqlType: "VARCHAR(16)"},
											{Name: "c_street_1", SqlType: "VARCHAR(20)"},
											{Name: "c_street_2", SqlType: "VARCHAR(20)"},
											{Name: "c_city", SqlType: "VARCHAR(20)"},
											{Name: "c_state", SqlType: "CHAR(2)"},
											{Name: "c_zip", SqlType: "CHAR(9)"},
											{Name: "c_phone", SqlType: "CHAR(16)"},
											{Name: "c_since", SqlType: "TIMESTAMP"},
											{Name: "c_credit", SqlType: "CHAR(2)"},
											{Name: "c_credit_lim", SqlType: "DECIMAL(12,2)"},
											{Name: "c_discount", SqlType: "DECIMAL(4,4)"},
											{Name: "c_balance", SqlType: "DECIMAL(12,2)"},
											{Name: "c_ytd_payment", SqlType: "DECIMAL(12,2)"},
											{Name: "c_payment_cnt", SqlType: "INTEGER"},
											{Name: "c_delivery_cnt", SqlType: "INTEGER"},
											{Name: "c_data", SqlType: "VARCHAR(500)"},
										},
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name:       "history",
										DbSpecific: &stroppy.Value_Struct{},
										Columns: []*stroppy.ColumnDescriptor{
											{Name: "h_c_id", SqlType: "INTEGER"},
											{Name: "h_c_d_id", SqlType: "INTEGER"},
											{Name: "h_c_w_id", SqlType: "INTEGER"},
											{Name: "h_d_id", SqlType: "INTEGER"},
											{Name: "h_w_id", SqlType: "INTEGER"},
											{Name: "h_date", SqlType: "TIMESTAMP"},
											{Name: "h_amount", SqlType: "DECIMAL(6,2)"},
											{Name: "h_data", SqlType: "VARCHAR(24)"},
										},
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name:       "new_order",
										DbSpecific: &stroppy.Value_Struct{},
										Columns: []*stroppy.ColumnDescriptor{
											{
												Name:       "no_o_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{
												Name:       "no_d_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{
												Name:       "no_w_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
												Constraint: ptr("REFERENCES warehouse(w_id)"),
											},
										},
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name:       "orders",
										DbSpecific: &stroppy.Value_Struct{},
										Columns: []*stroppy.ColumnDescriptor{
											{
												Name:       "o_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{
												Name:       "o_d_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{
												Name:       "o_w_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
												Constraint: ptr("REFERENCES warehouse(w_id)"),
											},
											{Name: "o_c_id", SqlType: "INTEGER"},
											{Name: "o_entry_d", SqlType: "TIMESTAMP"},
											{
												Name:     "o_carrier_id",
												SqlType:  "INTEGER",
												Nullable: ptr(true),
											},
											{Name: "o_ol_cnt", SqlType: "INTEGER"},
											{Name: "o_all_local", SqlType: "INTEGER"},
										},
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name:       "order_line",
										DbSpecific: &stroppy.Value_Struct{},
										Columns: []*stroppy.ColumnDescriptor{
											{
												Name:       "ol_o_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{
												Name:       "ol_d_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{
												Name:       "ol_w_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
												Constraint: ptr("REFERENCES warehouse(w_id)"),
											},
											{
												Name:       "ol_number",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{Name: "ol_i_id", SqlType: "INTEGER"},
											{Name: "ol_supply_w_id", SqlType: "INTEGER"},
											{
												Name:     "ol_delivery_d",
												SqlType:  "TIMESTAMP",
												Nullable: ptr(true),
											},
											{Name: "ol_quantity", SqlType: "INTEGER"},
											{Name: "ol_amount", SqlType: "DECIMAL(6,2)"},
											{Name: "ol_dist_info", SqlType: "CHAR(24)"},
										},
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name:       "item",
										DbSpecific: &stroppy.Value_Struct{},
										Columns: []*stroppy.ColumnDescriptor{
											{
												Name:       "i_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
											},
											{Name: "i_im_id", SqlType: "INTEGER"},
											{Name: "i_name", SqlType: "VARCHAR(24)"},
											{Name: "i_price", SqlType: "DECIMAL(5,2)"},
											{Name: "i_data", SqlType: "VARCHAR(50)"},
										},
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_CreateTable{
									CreateTable: &stroppy.TableDescriptor{
										Name:       "stock",
										DbSpecific: &stroppy.Value_Struct{},
										Columns: []*stroppy.ColumnDescriptor{
											{
												Name:       "s_i_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
												Constraint: ptr("REFERENCES item(i_id)"),
											},
											{
												Name:       "s_w_id",
												SqlType:    "INTEGER",
												PrimaryKey: ptr(true),
												Constraint: ptr("REFERENCES warehouse(w_id)"),
											},
											{Name: "s_quantity", SqlType: "INTEGER"},
											{Name: "s_dist_01", SqlType: "CHAR(24)"},
											{Name: "s_dist_02", SqlType: "CHAR(24)"},
											{Name: "s_dist_03", SqlType: "CHAR(24)"},
											{Name: "s_dist_04", SqlType: "CHAR(24)"},
											{Name: "s_dist_05", SqlType: "CHAR(24)"},
											{Name: "s_dist_06", SqlType: "CHAR(24)"},
											{Name: "s_dist_07", SqlType: "CHAR(24)"},
											{Name: "s_dist_08", SqlType: "CHAR(24)"},
											{Name: "s_dist_09", SqlType: "CHAR(24)"},
											{Name: "s_dist_10", SqlType: "CHAR(24)"},
											{Name: "s_ytd", SqlType: "INTEGER"},
											{Name: "s_order_cnt", SqlType: "INTEGER"},
											{Name: "s_remote_cnt", SqlType: "INTEGER"},
											{Name: "s_data", SqlType: "VARCHAR(50)"},
										},
									},
								},
							},
							Count: 1,
						},
					},
				},
				{
					Name: "create_stored_procedures",
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "create_dbms_random",
										Sql:  "CREATE OR REPLACE FUNCTION DBMS_RANDOM (INTEGER, INTEGER) RETURNS INTEGER AS $$\nDECLARE\n  start_int ALIAS FOR $1;\n  end_int ALIAS FOR $2;\nBEGIN\n  RETURN trunc(random() * (end_int-start_int + 1) + start_int);\nEND;\n$$ LANGUAGE 'plpgsql' STRICT;\n",
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "create_neword_procedure",
										Sql:  "CREATE OR REPLACE FUNCTION NEWORD (\n  no_w_id INTEGER,\n  no_max_w_id INTEGER,\n  no_d_id INTEGER,\n  no_c_id INTEGER,\n  no_o_ol_cnt INTEGER,\n  no_d_next_o_id INTEGER\n) RETURNS NUMERIC AS $$\nDECLARE\n  no_c_discount NUMERIC;\n  no_c_last VARCHAR;\n  no_c_credit VARCHAR;\n  no_d_tax NUMERIC;\n  no_w_tax NUMERIC;\n  no_s_quantity NUMERIC;\n  no_o_all_local SMALLINT;\n  rbk SMALLINT;\n  item_id_array INT[];\n  supply_wid_array INT[];\n  quantity_array SMALLINT[];\n  order_line_array SMALLINT[];\n  stock_dist_array CHAR(24)[];\n  s_quantity_array SMALLINT[];\n  price_array NUMERIC(5,2)[];\n  amount_array NUMERIC(5,2)[];\nBEGIN\n  no_o_all_local := 1;\n  SELECT c_discount, c_last, c_credit, w_tax\n  INTO no_c_discount, no_c_last, no_c_credit, no_w_tax\n  FROM customer, warehouse\n  WHERE warehouse.w_id = no_w_id AND customer.c_w_id = no_w_id\n    AND customer.c_d_id = no_d_id AND customer.c_id = no_c_id;\n\n  --#2.4.1.4\n  rbk := round(DBMS_RANDOM(1,100));\n\n  --#2.4.1.5\n  FOR loop_counter IN 1 .. no_o_ol_cnt\n  LOOP\n    IF ((loop_counter = no_o_ol_cnt) AND (rbk = 1))\n    THEN\n      item_id_array[loop_counter] := 100001;\n    ELSE\n      item_id_array[loop_counter] := round(DBMS_RANDOM(1,100000));\n    END IF;\n\n    --#2.4.1.5.2\n    IF ( round(DBMS_RANDOM(1,100)) > 1 )\n    THEN\n      supply_wid_array[loop_counter] := no_w_id;\n    ELSE\n      no_o_all_local := 0;\n      supply_wid_array[loop_counter] := 1 + MOD(CAST (no_w_id + round(DBMS_RANDOM(0,no_max_w_id-1)) AS INT), no_max_w_id);\n    END IF;\n\n    --#2.4.1.5.3\n    quantity_array[loop_counter] := round(DBMS_RANDOM(1,10));\n    order_line_array[loop_counter] := loop_counter;\n  END LOOP;\n\n  UPDATE district SET d_next_o_id = d_next_o_id + 1\n  WHERE d_id = no_d_id AND d_w_id = no_w_id\n  RETURNING d_next_o_id - 1, d_tax INTO no_d_next_o_id, no_d_tax;\n\n  INSERT INTO ORDERS (o_id, o_d_id, o_w_id, o_c_id, o_entry_d, o_ol_cnt, o_all_local)\n  VALUES (no_d_next_o_id, no_d_id, no_w_id, no_c_id, current_timestamp, no_o_ol_cnt, no_o_all_local);\n\n  INSERT INTO NEW_ORDER (no_o_id, no_d_id, no_w_id)\n  VALUES (no_d_next_o_id, no_d_id, no_w_id);\n\n  -- Stock and order line processing (simplified for brevity)\n  -- Full implementation would include district-specific s_dist processing\n\n  RETURN no_d_next_o_id;\nEXCEPTION\n  WHEN serialization_failure OR deadlock_detected OR no_data_found\n  THEN ROLLBACK; RETURN -1;\nEND;\n$$ LANGUAGE 'plpgsql';\n",
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "create_payment_procedure",
										Sql:  "CREATE OR REPLACE FUNCTION PAYMENT (\n  p_w_id INTEGER,\n  p_d_id INTEGER,\n  p_c_w_id INTEGER,\n  p_c_d_id INTEGER,\n  p_c_id_in INTEGER,\n  byname INTEGER,\n  p_h_amount NUMERIC,\n  p_c_last_in VARCHAR\n) RETURNS INTEGER AS $$\nDECLARE\n  p_c_balance NUMERIC(12, 2);\n  p_c_credit CHAR(2);\n  p_c_last VARCHAR(16);\n  p_c_id INTEGER;\n  p_w_name VARCHAR(11);\n  p_d_name VARCHAR(11);\n  name_count SMALLINT;\n  h_data VARCHAR(30);\nBEGIN\n  p_c_id := p_c_id_in;\n  p_c_last := p_c_last_in;\n\n  UPDATE warehouse\n  SET w_ytd = w_ytd + p_h_amount\n  WHERE w_id = p_w_id\n  RETURNING w_name INTO p_w_name;\n\n  UPDATE district\n  SET d_ytd = d_ytd + p_h_amount\n  WHERE d_w_id = p_w_id AND d_id = p_d_id\n  RETURNING d_name INTO p_d_name;\n\n  IF ( byname = 1 )\n  THEN\n    SELECT count(c_last) INTO name_count\n    FROM customer\n    WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id;\n\n    -- Get middle customer (simplified)\n    SELECT c_id, c_balance, c_credit\n    INTO p_c_id, p_c_balance, p_c_credit\n    FROM customer\n    WHERE c_last = p_c_last AND c_d_id = p_c_d_id AND c_w_id = p_c_w_id\n    ORDER BY c_first\n    LIMIT 1 OFFSET (name_count / 2);\n  ELSE\n    SELECT c_balance, c_credit\n    INTO p_c_balance, p_c_credit\n    FROM customer\n    WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;\n  END IF;\n\n  h_data := p_w_name || ' ' || p_d_name;\n\n  -- Update customer balance\n  UPDATE customer\n  SET c_balance = c_balance - p_h_amount,\n      c_ytd_payment = c_ytd_payment + p_h_amount,\n      c_payment_cnt = c_payment_cnt + 1\n  WHERE c_w_id = p_c_w_id AND c_d_id = p_c_d_id AND c_id = p_c_id;\n\n  INSERT INTO history (h_c_d_id, h_c_w_id, h_c_id, h_d_id, h_w_id, h_date, h_amount, h_data)\n  VALUES (p_c_d_id, p_c_w_id, p_c_id, p_d_id, p_w_id, current_timestamp, p_h_amount, h_data);\n\n  RETURN p_c_id;\nEXCEPTION\n  WHEN serialization_failure OR deadlock_detected OR no_data_found\n  THEN ROLLBACK; RETURN -1;\nEND;\n$$ LANGUAGE 'plpgsql';\n",
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "create_delivery_procedure",
										Sql:  "CREATE OR REPLACE FUNCTION DELIVERY (\n  d_w_id INTEGER,\n  d_o_carrier_id INTEGER\n) RETURNS INTEGER AS $$\nDECLARE\n  loop_counter SMALLINT;\n  d_id_in_array SMALLINT[] := ARRAY[1,2,3,4,5,6,7,8,9,10];\n  d_id_array SMALLINT[];\n  o_id_array INT[];\n  c_id_array INT[];\n  sum_amounts NUMERIC[];\nBEGIN\n  -- Delete from new_order and get order IDs\n  WITH new_order_delete AS (\n    DELETE FROM new_order as del_new_order\n    USING UNNEST(d_id_in_array) AS d_ids\n    WHERE no_d_id = d_ids\n      AND no_w_id = d_w_id\n      AND del_new_order.no_o_id = (\n        select min(select_new_order.no_o_id)\n        from new_order as select_new_order\n        where no_d_id = d_ids and no_w_id = d_w_id\n      )\n    RETURNING del_new_order.no_o_id, del_new_order.no_d_id\n  )\n  SELECT array_agg(no_o_id), array_agg(no_d_id)\n  FROM new_order_delete\n  INTO o_id_array, d_id_array;\n\n  -- Update orders with carrier\n  UPDATE orders\n  SET o_carrier_id = d_o_carrier_id\n  FROM UNNEST(o_id_array, d_id_array) AS ids(o_id, d_id)\n  WHERE orders.o_id = ids.o_id\n    AND o_d_id = ids.d_id\n    AND o_w_id = d_w_id;\n\n  -- Update order lines and get amounts\n  WITH order_line_update AS (\n    UPDATE order_line\n    SET ol_delivery_d = current_timestamp\n    FROM UNNEST(o_id_array, d_id_array) AS ids(o_id, d_id)\n    WHERE ol_o_id = ids.o_id\n      AND ol_d_id = ids.d_id\n      AND ol_w_id = d_w_id\n    RETURNING ol_d_id, ol_o_id, ol_amount\n  )\n  SELECT array_agg(ol_d_id), array_agg(c_id), array_agg(sum_amount)\n  FROM (\n    SELECT ol_d_id,\n           (SELECT DISTINCT o_c_id FROM orders WHERE o_id = ol_o_id AND o_d_id = ol_d_id AND o_w_id = d_w_id) AS c_id,\n           sum(ol_amount) AS sum_amount\n    FROM order_line_update\n    GROUP BY ol_d_id, ol_o_id\n  ) AS inner_sum\n  INTO d_id_array, c_id_array, sum_amounts;\n\n  -- Update customer balances\n  UPDATE customer\n  SET c_balance = COALESCE(c_balance,0) + ids_and_sums.sum_amounts,\n      c_delivery_cnt = c_delivery_cnt + 1\n  FROM UNNEST(d_id_array, c_id_array, sum_amounts) AS ids_and_sums(d_id, c_id, sum_amounts)\n  WHERE customer.c_id = ids_and_sums.c_id\n    AND c_d_id = ids_and_sums.d_id\n    AND c_w_id = d_w_id;\n\n  RETURN 1;\nEXCEPTION\n  WHEN serialization_failure OR deadlock_detected OR no_data_found\n  THEN ROLLBACK; RETURN -1;\nEND;\n$$ LANGUAGE 'plpgsql';\n",
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "create_ostat_procedure",
										Sql:  "CREATE OR REPLACE FUNCTION OSTAT (\n  os_w_id INTEGER,\n  os_d_id INTEGER,\n  os_c_id INTEGER,\n  byname INTEGER,\n  os_c_last VARCHAR\n) RETURNS TABLE(customer_info TEXT, order_info TEXT) AS $$\nDECLARE\n  namecnt INTEGER;\n  os_c_balance NUMERIC;\n  os_c_first VARCHAR;\n  os_c_middle VARCHAR;\n  os_o_id INTEGER;\n  os_entdate TIMESTAMP;\n  os_o_carrier_id INTEGER;\nBEGIN\n  IF ( byname = 1 )\n  THEN\n    SELECT count(c_id) INTO namecnt\n    FROM customer\n    WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id;\n\n    SELECT c_balance, c_first, c_middle, c_id\n    INTO os_c_balance, os_c_first, os_c_middle, os_c_id\n    FROM customer\n    WHERE c_last = os_c_last AND c_d_id = os_d_id AND c_w_id = os_w_id\n    ORDER BY c_first\n    LIMIT 1 OFFSET ((namecnt + 1) / 2);\n  ELSE\n    SELECT c_balance, c_first, c_middle, c_last\n    INTO os_c_balance, os_c_first, os_c_middle, os_c_last\n    FROM customer\n    WHERE c_id = os_c_id AND c_d_id = os_d_id AND c_w_id = os_w_id;\n  END IF;\n\n  SELECT o_id, o_carrier_id, o_entry_d\n  INTO os_o_id, os_o_carrier_id, os_entdate\n  FROM orders\n  WHERE o_d_id = os_d_id AND o_w_id = os_w_id AND o_c_id = os_c_id\n  ORDER BY o_id DESC\n  LIMIT 1;\n\n  RETURN QUERY SELECT\n    CAST(os_c_id || '|' || os_c_first || '|' || os_c_middle || '|' || os_c_balance AS TEXT) as customer_info,\n    CAST(os_o_id || '|' || os_o_carrier_id || '|' || os_entdate AS TEXT) as order_info;\n\nEXCEPTION\n  WHEN serialization_failure OR deadlock_detected OR no_data_found\n  THEN RETURN;\nEND;\n$$ LANGUAGE 'plpgsql';\n",
									},
								},
							},
							Count: 1,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "create_slev_procedure",
										Sql: `CREATE OR REPLACE FUNCTION SLEV (
  st_w_id INTEGER,
  st_d_id INTEGER,
  threshold INTEGER
) RETURNS INTEGER AS $$
DECLARE
  stock_count INTEGER;
BEGIN
  SELECT COUNT(DISTINCT (s_i_id)) INTO stock_count
  FROM order_line, stock, district
  WHERE ol_w_id = st_w_id
    AND ol_d_id = st_d_id
    AND d_w_id = st_w_id
    AND d_id = st_d_id
    AND (ol_o_id < d_next_o_id)
    AND ol_o_id >= (d_next_o_id - 20)
    AND s_w_id = st_w_id
    AND s_i_id = ol_i_id
    AND s_quantity < threshold;

  RETURN stock_count;
EXCEPTION
  WHEN serialization_failure OR deadlock_detected OR no_data_found
  THEN RETURN -1;
END;
$$ LANGUAGE 'plpgsql';
`,
									},
								},
							},
							Count: 1,
						},
					},
				},
				{
					Name: "load_data",
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Insert{
									Insert: &stroppy.InsertDescriptor{
										Name:      "load_items",
										TableName: "item",
										Method:    stroppy.InsertMethod_COPY_FROM.Enum(),
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "i_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: itemsMax,
														},
													},
													Unique: ptr(true),
												},
											},
											{
												Name: "i_im_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: itemsMax,
														},
													},
												},
											},
											{
												Name: "i_name",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](14),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "i_price",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_FloatRange{
														FloatRange: &stroppy.Generation_Range_Float{
															Min: ptr[float32](1),
															Max: 100,
														},
													},
												},
											},
											{
												Name: "i_data",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](26),
															MaxLen: 50,
														},
													},
												},
											},
										},
									},
								},
							},
							Count: uint64(itemsMax),
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Insert{
									Insert: &stroppy.InsertDescriptor{
										Name:      "load_warehouses",
										TableName: "warehouse",
										Method:    stroppy.InsertMethod_COPY_FROM.Enum(),
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "w_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: warehouseMax,
														},
													},
													Unique: ptr(true),
												},
											},
											{
												Name: "w_name",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															MinLen: ptr[uint64](6),
															MaxLen: 10,
														},
													},
												},
											},
											{
												Name: "w_street_1",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															MinLen: ptr[uint64](10),
															MaxLen: 20,
														},
													},
												},
											},
											{
												Name: "w_street_2",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															MinLen: ptr[uint64](10),
															MaxLen: 20,
														},
													},
												},
											},
											{
												Name: "w_city",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															MinLen: ptr[uint64](10),
															MaxLen: 20,
														},
													},
												},
											},
											{
												Name: "w_state",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															MinLen: ptr[uint64](2),
															MaxLen: 2,
														},
													},
												},
											},
											{
												Name: "w_zip",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](9),
															MaxLen: 9,
														},
													},
												},
											},
											{
												Name: "w_tax",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_FloatRange{
														FloatRange: &stroppy.Generation_Range_Float{
															Max: 0.20000000298023224,
														},
													},
												},
											},
											{
												Name: "w_ytd",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_FloatConst{
														FloatConst: 300000.00,
													},
												},
											},
										},
									},
								},
							},
							Count: uint64(warehouseMax),
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Insert{
									Insert: &stroppy.InsertDescriptor{
										Name:      "load_districts",
										TableName: "district",
										Method:    stroppy.InsertMethod_COPY_FROM.Enum(),
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "d_name",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																},
															},
															MinLen: ptr[uint64](6),
															MaxLen: 10,
														},
													},
												},
											},
											{
												Name: "d_street_1",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](10),
															MaxLen: 20,
														},
													},
												},
											},
											{
												Name: "d_street_2",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](10),
															MaxLen: 20,
														},
													},
												},
											},
											{
												Name: "d_city",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](10),
															MaxLen: 20,
														},
													},
												},
											},
											{
												Name: "d_state",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																},
															},
															MinLen: ptr[uint64](2),
															MaxLen: 2,
														},
													},
												},
											},
											{
												Name: "d_zip",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](9),
															MaxLen: 9,
														},
													},
												},
											},
											{
												Name: "d_tax",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_FloatRange{
														FloatRange: &stroppy.Generation_Range_Float{
															Max: 0.20000000298023224,
														},
													},
												},
											},
											{
												Name: "d_ytd",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_FloatConst{
														FloatConst: 30000.00,
													},
												},
											},
											{
												Name: "d_next_o_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Const{
														Int32Const: 3001,
													},
												},
											},
										},
										Groups: []*stroppy.QueryParamGroup{{
											Name: "district_pk",
											Params: []*stroppy.QueryParamDescriptor{
												{
													Name: "d_w_id",
													GenerationRule: &stroppy.Generation_Rule{
														Kind: &stroppy.Generation_Rule_Int32Range{
															Int32Range: &stroppy.Generation_Range_Int32{
																Min: ptr[int32](1),
																Max: warehouseMax,
															},
														},
														Unique: ptr(true),
													},
												},
												{
													Name: "d_id",
													GenerationRule: &stroppy.Generation_Rule{
														Kind: &stroppy.Generation_Rule_Int32Range{
															Int32Range: &stroppy.Generation_Range_Int32{
																Min: ptr[int32](1),
																Max: 10,
															},
														},
														Unique: ptr(true),
													},
												},
											},
										}},
									},
								},
							},
							Count: uint64(warehouseMax) * 10,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Insert{
									Insert: &stroppy.InsertDescriptor{
										Name:      "load_customers",
										TableName: "customer",
										Method:    stroppy.InsertMethod_COPY_FROM.Enum(),
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "c_first",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																},
															},
															MinLen: ptr[uint64](8),
															MaxLen: 16,
														},
													},
												},
											},
											{
												Name: "c_middle",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringConst{
														StringConst: "OE",
													},
												},
											},
											{
												Name: "c_last",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															MinLen: ptr[uint64](6),
															MaxLen: 16,
														},
													},
													Unique: ptr(true),
												},
											},
											{
												Name: "c_street_1",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](10),
															MaxLen: 20,
														},
													},
												},
											},
											{
												Name: "c_street_2",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](10),
															MaxLen: 20,
														},
													},
												},
											},
											{
												Name: "c_city",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](10),
															MaxLen: 20,
														},
													},
												},
											},
											{
												Name: "c_state",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																},
															},
															MinLen: ptr[uint64](2),
															MaxLen: 2,
														},
													},
												},
											},
											{
												Name: "c_zip",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringConst{
														StringConst: "123456789",
													},
												},
											},
											{
												Name: "c_phone",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](16),
															MaxLen: 16,
														},
													},
												},
											},
											{
												Name: "c_since",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_DatetimeConst{
														DatetimeConst: &stroppy.DateTime{
															Value: timestamppb.Now(), // TODO: add now()
														},
													},
												},
											},
											{
												Name: "c_credit",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringConst{
														StringConst: "GC",
													},
												},
											},
											{
												Name: "c_credit_lim",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_FloatConst{
														FloatConst: 50000.00,
													},
												},
											},
											{
												Name: "c_discount",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_FloatRange{
														FloatRange: &stroppy.Generation_Range_Float{
															Max: 0.5,
														},
													},
												},
											},
											{
												Name: "c_balance",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_FloatConst{
														FloatConst: -10.00,
													},
												},
											},
											{
												Name: "c_ytd_payment",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_FloatConst{
														FloatConst: 10.00,
													},
												},
											},
											{
												Name: "c_payment_cnt",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Const{
														Int32Const: 1,
													},
												},
											},
											{
												Name: "c_delivery_cnt",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Const{
														Int32Const: 0,
													},
												},
											},
											{
												Name: "c_data",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](300),
															MaxLen: 500,
														},
													},
												},
											},
										},
										Groups: []*stroppy.QueryParamGroup{{
											Name: "customer_pk",
											Params: []*stroppy.QueryParamDescriptor{
												{
													Name: "c_d_id",
													GenerationRule: &stroppy.Generation_Rule{
														Kind: &stroppy.Generation_Rule_Int32Range{
															Int32Range: &stroppy.Generation_Range_Int32{
																Min: ptr[int32](1),
																Max: 10,
															},
														},
														Unique: ptr(true),
													},
												},
												{
													Name: "c_w_id",
													GenerationRule: &stroppy.Generation_Rule{
														Kind: &stroppy.Generation_Rule_Int32Range{
															Int32Range: &stroppy.Generation_Range_Int32{
																Min: ptr[int32](1),
																Max: warehouseMax,
															},
														},
														Unique: ptr(true),
													},
												},
												{
													Name: "c_id",
													GenerationRule: &stroppy.Generation_Rule{
														Kind: &stroppy.Generation_Rule_Int32Range{
															Int32Range: &stroppy.Generation_Range_Int32{
																Min: ptr[int32](1),
																Max: 3000,
															},
														},
														Unique: ptr(true),
													},
												},
											},
										}},
									},
								},
							},
							Count: 3000 * 10 * uint64(warehouseMax),
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Insert{
									Insert: &stroppy.InsertDescriptor{
										Name:      "load_stock",
										TableName: "stock",
										Method:    stroppy.InsertMethod_COPY_FROM.Enum(),
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "s_quantity",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](10),
															Max: 100,
														},
													},
												},
											},
											{
												Name: "s_dist_01",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_dist_02",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_dist_03",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_dist_04",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_dist_05",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_dist_06",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_dist_07",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_dist_08",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_dist_09",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_dist_10",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																},
															},
															MinLen: ptr[uint64](24),
															MaxLen: 24,
														},
													},
												},
											},
											{
												Name: "s_ytd",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Const{
														Int32Const: 0,
													},
												},
											},
											{
												Name: "s_order_cnt",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Const{
														Int32Const: 0,
													},
												},
											},
											{
												Name: "s_remote_cnt",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Const{
														Int32Const: 0,
													},
												},
											},
											{
												Name: "s_data",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															Alphabet: &stroppy.Generation_Alphabet{
																Ranges: []*stroppy.Generation_Range_UInt32{
																	{
																		Min: ptr[uint32](65),
																		Max: 90,
																	},
																	{
																		Min: ptr[uint32](97),
																		Max: 122,
																	},
																	{
																		Min: ptr[uint32](48),
																		Max: 57,
																	},
																	{
																		Min: ptr[uint32](32),
																		Max: 33,
																	},
																},
															},
															MinLen: ptr[uint64](26),
															MaxLen: 50,
														},
													},
												},
											},
										},
										Groups: []*stroppy.QueryParamGroup{{
											Name: "stock_pk",
											Params: []*stroppy.QueryParamDescriptor{
												{
													Name: "s_i_id",
													GenerationRule: &stroppy.Generation_Rule{
														Kind: &stroppy.Generation_Rule_Int32Range{
															Int32Range: &stroppy.Generation_Range_Int32{
																Min: ptr[int32](1),
																Max: itemsMax,
															},
														},
														Unique: ptr(true),
													},
												},
												{
													Name: "s_w_id",
													GenerationRule: &stroppy.Generation_Rule{
														Kind: &stroppy.Generation_Rule_Int32Range{
															Int32Range: &stroppy.Generation_Range_Int32{
																Min: ptr[int32](1),
																Max: warehouseMax,
															},
														},
														Unique: ptr(true),
													},
												},
											},
										}},
									},
								},
							},
							Count: uint64(warehouseMax) * uint64(itemsMax),
						},
					},
				},
				{
					Name:  "tpcc_workload",
					Async: ptr(true),
					Units: []*stroppy.WorkloadUnitDescriptor{
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "call_neword",
										Sql:  "SELECT NEWORD(${w_id}, ${max_w_id}, ${d_id}, ${c_id}, ${ol_cnt}, 0)",
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "w_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 10,
														},
													},
												},
											},
											{
												Name: "max_w_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](10),
															Max: 10,
														},
													},
												},
											},
											{
												Name: "d_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 10,
														},
													},
												},
											},
											{
												Name: "c_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 3000,
														},
													},
												},
											},
											{
												Name: "ol_cnt",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](5),
															Max: 15,
														},
													},
												},
											},
										},
									},
								},
							},
							Count: 45,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "payment_procedure",
										Sql:  "SELECT PAYMENT(${p_w_id}, ${p_d_id}, ${p_c_w_id}, ${p_c_d_id},\n${p_c_id}, ${byname}, ${h_amount}, ${c_last})",
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "p_w_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: warehouseMax,
														},
													},
												},
											},
											{
												Name: "p_d_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 10,
														},
													},
												},
											},
											{
												Name: "p_c_w_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: warehouseMax,
														},
													},
												},
											},
											{
												Name: "p_c_d_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 10,
														},
													},
												},
											},
											{
												Name: "p_c_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 3000,
														},
													},
												},
											},
											{
												Name: "byname",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](0),
															Max: 0,
														},
													},
												},
											},
											{
												Name: "h_amount",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_DoubleRange{
														DoubleRange: &stroppy.Generation_Range_Double{
															Min: ptr[float64](1),
															Max: 5000,
														},
													},
												},
											},
											{
												Name: "c_last",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															MinLen: ptr[uint64](6),
															MaxLen: 16,
														},
													},
													Unique: ptr(true),
												},
											},
										},
									},
								},
							},
							Count: 43,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "order_status_procedure",
										Sql:  "SELECT * FROM OSTAT(${os_w_id}, ${os_d_id}, ${os_c_id},\n${byname}, ${os_c_last})",
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "os_w_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: warehouseMax,
														},
													},
												},
											},
											{
												Name: "os_d_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 10,
														},
													},
												},
											},
											{
												Name: "os_c_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 3000,
														},
													},
												},
											},
											{
												Name: "byname",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](0),
															Max: 0,
														},
													},
												},
											},
											{
												Name: "os_c_last",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_StringRange{
														StringRange: &stroppy.Generation_Range_String{
															MinLen: ptr[uint64](8),
															MaxLen: 16,
														},
													},
												},
											},
										},
									},
								},
							},
							Count: 4,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "delivery_procedure",
										Sql:  "SELECT DELIVERY(${d_w_id}, ${d_o_carrier_id})\n",
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "d_w_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: warehouseMax,
														},
													},
												},
											},
											{
												Name: "d_o_carrier_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 10,
														},
													},
												},
											},
										},
									},
								},
							},
							Count: 4,
						},
						{
							Descriptor_: &stroppy.UnitDescriptor{
								Type: &stroppy.UnitDescriptor_Query{
									Query: &stroppy.QueryDescriptor{
										Name: "stock_level_transaction",
										Sql:  "SELECT SLEV(${st_w_id}, ${st_d_id}, ${threshold})\n",
										Params: []*stroppy.QueryParamDescriptor{
											{
												Name: "st_w_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: warehouseMax,
														},
													},
												},
											},
											{
												Name: "st_d_id",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](1),
															Max: 10,
														},
													},
												},
											},
											{
												Name: "threshold",
												GenerationRule: &stroppy.Generation_Rule{
													Kind: &stroppy.Generation_Rule_Int32Range{
														Int32Range: &stroppy.Generation_Range_Int32{
															Min: ptr[int32](10),
															Max: 20,
														},
													},
												},
											},
										},
									},
								},
							},
							Count: 4,
						},
					},
				},
				{
					Name: "cleanup",
					Units: []*stroppy.WorkloadUnitDescriptor{{
						Descriptor_: &stroppy.UnitDescriptor{
							Type: &stroppy.UnitDescriptor_Query{
								Query: &stroppy.QueryDescriptor{
									Name: "drop_procedures",
									Sql:  "DROP FUNCTION IF EXISTS neword, payment, delivery, DBMS_RANDOM CASCADE",
								},
							},
						},
						Count: 1,
					}, {
						Descriptor_: &stroppy.UnitDescriptor{
							Type: &stroppy.UnitDescriptor_Query{
								Query: &stroppy.QueryDescriptor{
									Name: "drop_tables",
									Sql:  "DROP TABLE IF EXISTS order_line, orders, new_order, history, customer, district, stock, item, warehouse CASCADE",
								},
							},
						},
						Count: 1,
					}},
				},
			},
		},
	}
}
