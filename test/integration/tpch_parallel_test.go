//go:build integration

package integration

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestTpchParallelLoadNoRace drives `workloads/tpch/tx.ts` at SF=0.1
// with LOAD_WORKERS=8 repeatedly. Before the LookupRegistry.Clone fix,
// tpch's lineitem reads through ordersLookup + partLookup from every
// parallel chunk tripped `fatal error: concurrent map writes` roughly
// half the time at workers ≥ 4. The test asserts every iteration
// completes without crash.
//
// Each iteration resets the schema and invokes stroppy end-to-end,
// so the total wall-clock is high — kept out of the default per-PR
// integration suite by the `integration` build tag. Set
// TPCH_PARALLEL_ITERATIONS to override the repeat count (default 10).
func TestTpchParallelLoadNoRace(t *testing.T) {
	if os.Getenv(envSkip) == "1" {
		t.Skipf("skipping integration test: %s=1", envSkip)
	}

	repoRoot := findRepoRoot(t)
	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found at %s (run `make build` first): %v", binary, err)
	}

	pool := NewTmpfsPG(t)

	url := os.Getenv(envTmpfsURL)
	if url == "" {
		url = defaultTmpfsURL
	}

	iterations := 10
	if raw := os.Getenv("TPCH_PARALLEL_ITERATIONS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			iterations = parsed
		}
	}

	for i := range iterations {
		// Reset the schema between runs: each iteration must be
		// independent so a flaky crash surfaces at the iteration
		// boundary rather than as a `table already exists` error.
		ResetSchema(t, pool)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)

		cmd := exec.CommandContext(ctx, binary,
			"run", "./workloads/tpch/tx.ts",
			"-D", "url="+url,
			"-e", "SCALE_FACTOR=0.1",
			"-e", "STROPPY_NO_DEFAULT=true",
			"--steps", "drop_schema,create_schema,load_data",
		)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), "LOAD_WORKERS=8")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		cancel()

		if err != nil {
			t.Fatalf("iter %d: stroppy load failed: %v\n--- stdout ---\n%s\n--- stderr ---\n%s",
				i, err, stdout.String(), stderr.String())
		}
	}
}
