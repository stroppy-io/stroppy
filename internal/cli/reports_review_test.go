package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestReportPruneEmptyAgeIntersection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	id := seedCLIReport(t, time.Now().UTC(), "recent", report.StatusCompleted)
	for _, yes := range []bool{false, true} {
		args := []string{
			"reports", "prune", "--older-than", "720h", "--since",
			time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339), "-o", "json",
		}
		if yes {
			args = append(args, "--yes")
		}

		stdout, _, err := reportCLI(t, args...)
		if err != nil || !strings.Contains(stdout, `"ids": []`) {
			t.Fatalf("empty intersection = %s, %v", stdout, err)
		}
	}

	if _, _, err := reportCLI(t, "reports", "get", id); err != nil {
		t.Fatalf("empty prune removed report: %v", err)
	}
}

func TestStandaloneReportHelp(t *testing.T) {
	test := bench.Test{Name: "standalone-help", Define: func(d *bench.Def) error { return d.Execution.Err() }}

	catalog, err := bench.NewCatalog(test)
	if err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{"list", "prune"} {
		var stdout, stderr bytes.Buffer

		err := Execute(t.Context(), &Options{Catalog: catalog, DefaultWorkload: test.Name},
			[]string{"reports", command, "--help"}, &stdout, &stderr)
		if err != nil || !strings.Contains(stdout.String(), "stroppy reports "+command) ||
			!strings.Contains(stdout.String(), "--workload") {
			t.Fatalf("standalone help = %s, %s, %v", stdout.String(), stderr.String(), err)
		}
	}
}
