//go:build integration && integration_sf1

package integration

import (
	"testing"
	"time"
)

// TestTpchAnswersSpotCheck runs the explicit heavy SF=1 validation suite.
func TestTpchAnswersSpotCheck(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	url := envOr(envTmpfsURL, defaultTmpfsURL)
	out := runStroppy(t, 60*time.Minute,
		"run", "tpch/tx",
		"-d", "pg",
		"-D", "url="+url,
		"--scale-factor", "1",
		"--load-workers", "8",
		"--executor", "shared-iterations",
		"--iterations", "1",
		"--steps", "drop_schema,create_schema,load_data,create_indexes,analyze,validate_answers",
	)

	if err := validateTpchSF1Summary(out); err != nil {
		t.Fatal(err)
	}
}
