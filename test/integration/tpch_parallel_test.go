//go:build integration

package integration

import (
	"testing"
	"time"
)

// TestTpchParallelLoadNoRace repeats the Go TPC-H load with eight workers.
// Two scale-0.01 runs keep the mandatory suite practical while exercising the
// parallel lookup path that previously crashed with concurrent map writes.
func TestTpchParallelLoadNoRace(t *testing.T) {
	pool := NewTmpfsPG(t)
	url := envOr(envTmpfsURL, defaultTmpfsURL)

	for i := range 2 {
		ResetSchema(t, pool)
		runStroppy(t, 3*time.Minute,
			"run", "tpch/tx",
			"-d", "pg",
			"-D", "url="+url,
			"--scale-factor", "0.01",
			"--load-workers", "8",
			"--executor", "shared-iterations",
			"--iterations", "1",
			"--steps", "drop_schema,create_schema,load_data",
		)
		t.Logf("parallel TPC-H load %d/2 completed", i+1)
	}
}
