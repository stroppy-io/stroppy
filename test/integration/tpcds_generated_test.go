//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTPCDSGeneratedPostgresAllQueries(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	reportFile := filepath.Join(t.TempDir(), "report.json")
	out := runStroppy(t, 5*time.Minute,
		"run", "tpcds", "-d", "pg", "-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL),
		"--scale-factor", "0.01", "--query-stream", "0", "--query-seed", "19620718",
		"--executor", "shared-iterations", "--iterations", "1", "--vus", "1",
		"--report-file", reportFile,
	)
	matches := regexp.MustCompile(`query completed\s+\{[^\n]*"query": "query(\d+)(?:_[abc])?"`).FindAllStringSubmatch(out, -1)
	if len(matches) != 103 {
		t.Fatalf("got %d successful SQL statements, want 103\n%s", len(matches), out)
	}
	seen := make(map[string]bool)
	for _, match := range matches {
		seen[match[1]] = true
	}
	for i := 1; i <= 99; i++ {
		if !seen[strconv.Itoa(i)] {
			t.Errorf("Q%d missing", i)
		}
	}
	raw, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Steps []struct {
			Name       string
			Executions int
		}
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, step := range envelope.Steps {
		if step.Name == "workload" {
			if step.Executions != 1 {
				t.Fatalf("preflight counted as a workload execution: %d", step.Executions)
			}
			return
		}
	}
	t.Fatal("workload step missing from report")
}

func TestTPCDSIncompleteMySQLFailsBeforeSchemaChanges(t *testing.T) {
	db := NewMySQL(t)
	ResetMySQL(t, db, []string{"customer"})
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE customer (marker INT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO customer VALUES (123)"); err != nil {
		t.Fatal(err)
	}
	repo, binary := stroppyBinary(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "run", "tpcds", "-d", "mysql",
		"-D", "url="+envOr(envMySQLAllURL, defaultMySQLAllURL),
		"--scale-factor", "0.01", "--query-stream", "0",
		"--executor", "shared-iterations", "--iterations", "1")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "generated query set is incomplete") {
		t.Fatalf("expected explicit preflight failure, got %v\n%s", err, out)
	}
	var marker int
	if err := db.QueryRowContext(t.Context(), "SELECT marker FROM customer").Scan(&marker); err != nil || marker != 123 {
		t.Fatalf("preflight modified the existing schema: marker=%d, err=%v", marker, err)
	}
}
