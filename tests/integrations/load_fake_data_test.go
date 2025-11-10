package integrations

import (
	"fmt"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"google.golang.org/protobuf/types/known/timestamppb"
	"math/rand"
	"testing"
	"time"
)

func TestLoadFakeData(t *testing.T) {
	ctx := registerUser(t)
	runClient := newRunClient()

	// Seed the random number generator
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	const numRuns = 100

	statuses := []panel.Status{
		panel.Status_STATUS_IDLE,
		panel.Status_STATUS_COMPLETED,
		panel.Status_STATUS_RUNNING,
		panel.Status_STATUS_FAILED,
		panel.Status_STATUS_CANCELED,
	}

	for i := 0; i < numRuns; i++ {
		// Generate random timestamps
		createdAt := time.Now().Add(-time.Duration(r.Intn(30*24)) * time.Hour) // last 30 days
		updatedAt := createdAt.Add(time.Duration(r.Intn(24)) * time.Hour)      // up to 24 hours after created

		// Generate random TPS values
		avgTps := uint64(r.Intn(5000) + 1000) // 1000-6000 TPS
		maxTps := avgTps + uint64(r.Intn(2000))
		minTps := avgTps - uint64(r.Intn(500))
		p95Tps := avgTps + uint64(r.Intn(1000))
		p99Tps := avgTps + uint64(r.Intn(1500))

		// Generate random machine specs
		dbCores := uint32(r.Intn(8) + 2)   // 2-9 cores
		dbMemory := uint32(r.Intn(28) + 4) // 4-31 GB
		dbDisk := uint32(r.Intn(91) + 10)  // 10-100 GB

		wlCores := uint32(r.Intn(8) + 2)   // 2-9 cores
		wlMemory := uint32(r.Intn(28) + 4) // 4-31 GB
		wlDisk := uint32(r.Intn(91) + 10)  // 10-100 GB

		// Random single machine mode
		dbSingleMode := r.Intn(2) == 0
		wlSingleMode := r.Intn(2) == 0

		// Random status
		status := statuses[r.Intn(len(statuses))]

		// Create database cluster configuration
		dbMachines := []*panel.MachineInfo{
			{
				Cores:  dbCores,
				Memory: dbMemory,
				Disk:   dbDisk,
			},
		}
		if !dbSingleMode {
			numMachines := r.Intn(3) + 2 // 2-4 machines
			for j := 1; j < numMachines; j++ {
				dbMachines = append(dbMachines, &panel.MachineInfo{
					Cores:  dbCores,
					Memory: dbMemory,
					Disk:   dbDisk,
				})
			}
		}

		// Create workload cluster configuration
		wlMachines := []*panel.MachineInfo{
			{
				Cores:  wlCores,
				Memory: wlMemory,
				Disk:   wlDisk,
			},
		}
		if !wlSingleMode {
			numMachines := r.Intn(3) + 2 // 2-4 machines
			for j := 1; j < numMachines; j++ {
				wlMachines = append(wlMachines, &panel.MachineInfo{
					Cores:  wlCores,
					Memory: wlMemory,
					Disk:   wlDisk,
				})
			}
		}

		runRecord := &panel.RunRecord{
			Timing: &panel.Timing{
				CreatedAt: timestamppb.New(createdAt),
				UpdatedAt: timestamppb.New(updatedAt),
			},
			Status: status,
			Tps: &panel.Tps{
				Average: avgTps,
				Max:     maxTps,
				Min:     minTps,
				P95Th:   p95Tps,
				P99Th:   p99Tps,
			},
			Database: &panel.Database{
				Name:         fmt.Sprintf("db_fake_%d", i+1),
				DatabaseType: panel.Database_TYPE_POSTGRES_ORIOLE,
				RunnerCluster: &panel.Cluster{
					IsSingleMachineMode: dbSingleMode,
					Machines:            dbMachines,
				},
			},
			Workload: &panel.Workload{
				Name:         fmt.Sprintf("workload_fake_%d", i+1),
				WorkloadType: panel.Workload_TYPE_TPCC,
				RunnerCluster: &panel.Cluster{
					IsSingleMachineMode: wlSingleMode,
					Machines:            wlMachines,
				},
			},
		}

		resp, err := runClient.AddRun(ctx, runRecord)
		if err != nil {
			t.Logf("Failed to add run %d: %v", i+1, err)
		} else {
			t.Logf("Run %d/%d added successfully: %v", i+1, numRuns, resp)
		}
	}

	t.Logf("Successfully attempted to add %d fake runs", numRuns)
}
