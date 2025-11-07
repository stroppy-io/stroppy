package api

import (
	"github.com/stretchr/testify/require"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
)

func TestAddRun(t *testing.T) {
	service, ctx, user := newDevTestService(t)
	createdRecord, err := service.AddRun(ctx, &panel.RunRecord{
		Timing: &panel.Timing{
			CreatedAt: timestamppb.Now(),
			UpdatedAt: timestamppb.Now(),
		},
		Tps: &panel.Tps{
			Average: 100,
			Max:     150,
			Min:     50,
			P95Th:   120,
			P99Th:   140,
		},
		Database: &panel.Database{
			Name:         "test_db",
			DatabaseType: panel.Database_TYPE_POSTGRES,
			Parameters:   map[string]string{"key": "value"},
		},
		Workload: &panel.Workload{
			Name:         "test_workload",
			WorkloadType: panel.Workload_TYPE_TPCC,
			Parameters:   map[string]string{"key": "value"},
		},
		Status: panel.Status_STATUS_COMPLETED,
	})
	require.NoError(t, err)
	require.NotNil(t, createdRecord.Id)
	require.NotNil(t, createdRecord.AuthorId)
	require.Equal(t, createdRecord.AuthorId.GetId(), user.GetId().GetId())
}

func TestListTopRuns(t *testing.T) {
	service, ctx, _ := newDevTestService(t)

	records, err := service.ListTopRuns(ctx, &emptypb.Empty{})
	require.Error(t, err, ErrTopRunsNotFound)
	require.Nil(t, records)

	_, err = service.AddRun(ctx, &panel.RunRecord{
		Timing: &panel.Timing{
			CreatedAt: timestamppb.Now(),
			UpdatedAt: timestamppb.Now(),
		},
		Status: panel.Status_STATUS_COMPLETED,
		Tps: &panel.Tps{
			Average: 100,
			Max:     150,
			Min:     50,
			P95Th:   120,
			P99Th:   140,
		},
		Database: &panel.Database{
			Name:         "test_db",
			DatabaseType: panel.Database_TYPE_POSTGRES,
			Parameters:   map[string]string{"key": "value"},
			RunnerCluster: &panel.Cluster{
				IsSingleMachineMode: true,
				Machines: []*panel.MachineInfo{
					{
						Cores:  4,
						Memory: 8,
						Disk:   64,
					},
				},
			},
		},
		Workload: &panel.Workload{
			Name:         "test_workload",
			WorkloadType: panel.Workload_TYPE_TPCC,
			Parameters:   map[string]string{"key": "value"},
			RunnerCluster: &panel.Cluster{
				IsSingleMachineMode: true,
				Machines: []*panel.MachineInfo{
					{
						Cores:  2,
						Memory: 4,
						Disk:   16,
					},
				},
			},
		},
	})
	require.NoError(t, err)

	records, err = service.ListTopRuns(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	require.NotNil(t, records)
	require.Len(t, records.Records, 1)
}

func TestListRuns(t *testing.T) {
	service, ctx, _ := newDevTestService(t)

	// Создаем тестовые данные
	_, err := service.AddRun(ctx, &panel.RunRecord{
		Timing: &panel.Timing{
			CreatedAt: timestamppb.Now(),
			UpdatedAt: timestamppb.Now(),
		},
		Tps: &panel.Tps{
			Average: 100,
			Max:     150,
			Min:     50,
			P95Th:   120,
			P99Th:   140,
		},
		Database: &panel.Database{
			Name:         "test_db_1",
			DatabaseType: panel.Database_TYPE_POSTGRES,
			Parameters:   map[string]string{"key": "value"},
			RunnerCluster: &panel.Cluster{
				IsSingleMachineMode: true,
				Machines: []*panel.MachineInfo{
					{
						Cores:  4,
						Memory: 8,
						Disk:   64,
					},
				},
			},
		},
		Workload: &panel.Workload{
			Name:         "test_workload_1",
			WorkloadType: panel.Workload_TYPE_TPCC,
			Parameters:   map[string]string{"key": "value"},
			RunnerCluster: &panel.Cluster{
				IsSingleMachineMode: true,
				Machines: []*panel.MachineInfo{
					{
						Cores:  2,
						Memory: 4,
						Disk:   16,
					},
				},
			},
		},
		Status: panel.Status_STATUS_COMPLETED,
	})
	require.NoError(t, err)

	_, err = service.AddRun(ctx, &panel.RunRecord{
		Timing: &panel.Timing{
			CreatedAt: timestamppb.Now(),
			UpdatedAt: timestamppb.Now(),
		},
		Tps: &panel.Tps{
			Average: 200,
			Max:     250,
			Min:     150,
			P95Th:   220,
			P99Th:   240,
		},
		Database: &panel.Database{
			Name:         "test_db_2",
			DatabaseType: panel.Database_TYPE_POSTGRES,
			Parameters:   map[string]string{"key": "value2"},
			RunnerCluster: &panel.Cluster{
				IsSingleMachineMode: true,
				Machines: []*panel.MachineInfo{
					{
						Cores:  8,
						Memory: 16,
						Disk:   128,
					},
				},
			},
		},
		Workload: &panel.Workload{
			Name:         "test_workload_2",
			WorkloadType: panel.Workload_TYPE_TPCC,
			Parameters:   map[string]string{"key": "value2"},
			RunnerCluster: &panel.Cluster{
				IsSingleMachineMode: true,
				Machines: []*panel.MachineInfo{
					{
						Cores:  4,
						Memory: 8,
						Disk:   32,
					},
				},
			},
		},
		Status: panel.Status_STATUS_RUNNING,
	})
	require.NoError(t, err)

	// Тест 1: Базовый запрос без фильтров
	t.Run("basic_request_no_filters", func(t *testing.T) {
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 2)
	})

	// Тест 2: Фильтр по лимиту
	t.Run("filter_by_limit", func(t *testing.T) {
		limit := int32(1)
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			Limit: &limit,
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1)
	})

	// Тест 3: Фильтр по offset
	t.Run("filter_by_offset", func(t *testing.T) {
		offset := int32(1)
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			Offset: &offset,
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1)
	})

	// Тест 4: Фильтр по статусу
	t.Run("filter_by_status", func(t *testing.T) {
		status := panel.Status_STATUS_COMPLETED
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			Status: &status,
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1)
	})

	// Тест 5: Фильтр по имени workload
	t.Run("filter_by_workload_name", func(t *testing.T) {
		workloadName := "test_workload_1"
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			WorkloadName: &workloadName,
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1)
	})

	// Тест 6: Фильтр по типу workload
	t.Run("filter_by_workload_type", func(t *testing.T) {
		workloadType := panel.Workload_TYPE_TPCC
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			WorkloadType: &workloadType,
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1)
	})

	// Тест 7: Фильтр по имени базы данных
	t.Run("filter_by_database_name", func(t *testing.T) {
		databaseName := "test_db_1"
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			DatabaseName: &databaseName,
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1)
	})

	// Тест 8: Фильтр по типу базы данных
	t.Run("filter_by_database_type", func(t *testing.T) {
		databaseType := panel.Database_TYPE_POSTGRES
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			DatabaseType: &databaseType,
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 2) // Оба используют POSTGRES
	})

	// Тест 9: Фильтр по TPS average
	t.Run("filter_by_tps_average", func(t *testing.T) {
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			TpsFilter: &panel.Tps_Filter{
				ParameterType: panel.Tps_Filter_TYPE_AVERAGE,
				Operator:      panel.NumberFilterOperator_TYPE_GREATER_THAN,
				Value:         150,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1) // Только запись с average = 200
	})

	// Тест 10: Фильтр по TPS max
	t.Run("filter_by_tps_max", func(t *testing.T) {
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			TpsFilter: &panel.Tps_Filter{
				ParameterType: panel.Tps_Filter_TYPE_MAX,
				Operator:      panel.NumberFilterOperator_TYPE_LESS_THAN,
				Value:         200,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1) // Только запись с max = 150
	})

	// Тест 11: Фильтр по машине - cores
	t.Run("filter_by_machine_cores", func(t *testing.T) {
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			MachineFilter: &panel.MachineInfo_Filter{
				ParameterType: panel.MachineInfo_Filter_TYPE_CORES,
				Operator:      panel.NumberFilterOperator_TYPE_GREATER_THAN,
				Value:         2,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 2) // Оба имеют cores > 2
	})

	// Тест 12: Фильтр по машине - memory
	t.Run("filter_by_machine_memory", func(t *testing.T) {
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			MachineFilter: &panel.MachineInfo_Filter{
				ParameterType: panel.MachineInfo_Filter_TYPE_MEMORY,
				Operator:      panel.NumberFilterOperator_TYPE_EQUAL,
				Value:         8,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1) // Только первая запись
	})

	// Тест 13: Комбинированные фильтры
	t.Run("combined_filters", func(t *testing.T) {
		workloadType := panel.Workload_TYPE_TPCC
		limit := int32(10)
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			WorkloadType: &workloadType,
			Limit:        &limit,
		})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.Len(t, records.Records, 1)
	})

	// Тест 14: Пустой результат
	t.Run("empty_result", func(t *testing.T) {
		workloadName := "nonexistent_workload"
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			WorkloadName: &workloadName,
		})
		require.Error(t, err)
		require.Nil(t, records)
		require.Equal(t, ErrRunsNotFound, err)
	})

	// Тест 15: Неверный TPS фильтр
	t.Run("invalid_tps_filter", func(t *testing.T) {
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			TpsFilter: &panel.Tps_Filter{
				ParameterType: panel.Tps_Filter_TYPE_UNSPECIFIED,
				Operator:      panel.NumberFilterOperator_TYPE_EQUAL,
				Value:         100,
			},
		})
		require.Error(t, err)
		require.Nil(t, records)
		require.Equal(t, ErrInvalidTpsFilter, err)
	})

	// Тест 16: Неверный machine фильтр
	t.Run("invalid_machine_filter", func(t *testing.T) {
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{
			MachineFilter: &panel.MachineInfo_Filter{
				ParameterType: panel.MachineInfo_Filter_TYPE_UNSPECIFIED,
				Operator:      panel.NumberFilterOperator_TYPE_EQUAL,
				Value:         4,
			},
		})
		require.Error(t, err)
		require.Nil(t, records)
		require.Equal(t, ErrInvalidMachineFilter, err)
	})

	// Тест 17: Проверка структуры результата
	t.Run("result_structure", func(t *testing.T) {
		limit := int32(1)
		records, err := service.ListRuns(ctx, &panel.ListRunsRequest{Limit: &limit})
		require.NoError(t, err)
		require.NotNil(t, records)
		require.NotNil(t, records.Records)
		require.Len(t, records.Records, 1)

		record := records.Records[0]
		require.NotNil(t, record.Id)
		require.NotNil(t, record.AuthorId)
		require.NotNil(t, record.Timing)
		require.NotNil(t, record.Tps)
		require.NotNil(t, record.Database)
		require.NotNil(t, record.Workload)
		require.NotNil(t, record.Status)
	})
}
