package tpcds

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestBenchmarkOrdering(t *testing.T) {
	queries := make([]namedQuery, 99)
	for i := range queries {
		queries[i].name = "query_" + strconv.Itoa(i+1)
	}

	expected := [][]string{
		{"96", "7", "75", "44", "39"},
		{"83", "32", "30", "92", "66"},
		{"56", "98", "59", "24", "88"},
	}
	for stream, names := range expected {
		ordered, err := benchmarkQueryOrder(queries, stream)
		if err != nil {
			t.Fatal(err)
		}

		for i, want := range names {
			if ordered[i].name != "query_"+want {
				t.Fatalf("stream %d query %d got %s want %s", stream, i, ordered[i].name, want)
			}
		}
	}
}

func TestBenchmarkDefinition(t *testing.T) {
	for _, driver := range []bench.DriverTypeName{bench.DriverPostgres, bench.DriverMySQL} {
		catalog, err := bench.NewCatalog(Test)
		if err != nil {
			t.Fatal(err)
		}

		inputs := bench.ParamInputs{CLI: map[string]string{"benchmark": "true"}}

		_, err = catalog.Resolve("tpcds", inputs, map[string]bench.DriverConfig{"": {Kind: driver}})
		if err != nil {
			t.Fatal(driver, err)
		}
	}
}

func TestBenchmarkInvalidSelection(t *testing.T) {
	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, extra := range []map[string]string{
		{"streams": "3"},
		{"streams": "2"},
		{"vus": "4"},
		{"iterations": "2"},
		{"executor": "constant-vus"},
		{"pg-unlogged": "true"},
		{"query-stream": "0"},
		{"refresh-set": "2"},
		{"scale-factor": "NaN"},
		{"scale-factor": "100001"},
	} {
		values := map[string]string{"benchmark": "true"}
		for key, value := range extra {
			values[key] = value
		}

		_, err := catalog.Resolve("tpcds", bench.ParamInputs{CLI: values},
			map[string]bench.DriverConfig{"": {Kind: bench.DriverPostgres}})
		if err == nil {
			t.Fatalf("accepted %v", extra)
		}
	}
}

func TestBenchmarkContinuationSelection(t *testing.T) {
	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, selection := range []struct {
		noSteps []string
		valid   bool
	}{
		{[]string{"load_data"}, false},
		{[]string{"load_data", "drop_schema"}, false},
		{[]string{"load_data", "create_schema"}, false},
		{[]string{"load_data", "drop_schema", "create_schema"}, true},
	} {
		_, err := catalog.ResolveRun("tpcds", bench.RunOptions{
			Drivers: map[string]bench.DriverConfig{"": {Kind: bench.DriverPostgres}},
			Params: bench.ParamInputs{CLI: map[string]string{
				"benchmark": "true", "refresh-set": "3",
			}},
			NoSteps: selection.noSteps,
		})
		if (err == nil) != selection.valid {
			t.Fatalf("no-steps %v: %v", selection.noSteps, err)
		}
	}
}

func benchmarkReportFixture() (*workload, bench.ReportContext) {
	w := &workload{streams: 4, benchmark: &benchmarkState{ScaleFactor: 1, Sq: 4, FirstSet: 1, NextSet: 5}}

	for _, name := range []string{"power", "throughput_1", "maintenance_1", "throughput_2", "maintenance_2"} {
		phase := &benchmarkPhase{Name: name, Status: string(bench.Completed), Seconds: 3600}
		if strings.HasPrefix(name, "throughput") || name == "power" {
			count := 4
			if name == "power" {
				count = 1
			}

			phase.Streams = make([]benchmarkStream, count)
			for i := range phase.Streams {
				phase.Streams[i] = benchmarkStream{Complete: true, Queries: make([]benchmarkQuery, 103)}
			}
		}

		w.benchmark.Phases = append(w.benchmark.Phases, phase)
	}

	for range 44 {
		w.benchmark.Attempts = append(w.benchmark.Attempts, maintenanceAttempt{Status: "completed"})
	}

	snapshot := bench.ReportContext{Status: report.StatusCompleted}
	for _, name := range []string{"create_schema", "load_data", "create_indexes", "analyze", "prepare_maintenance"} {
		snapshot.Steps = append(snapshot.Steps, report.Step{
			Name: name, Status: string(bench.Completed), DurationSeconds: 900,
		})
	}

	return w, snapshot
}

func TestBenchmarkContribution(t *testing.T) {
	w, snapshot := benchmarkReportFixture()

	contribution, err := w.benchmarkContribution(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	value := contribution.Data.(benchmarkState)
	if !value.SequenceValid || value.Score == nil || *value.Score != 418 || value.TPCCompliant {
		t.Fatalf("score: %+v", value)
	}

	if value.TLD != .05 || value.TPT != 4 || value.TTT != 2 || value.TDM != 2 {
		t.Fatalf("terms: %+v", value)
	}

	for _, change := range []func(*workload, *bench.ReportContext){
		func(_ *workload, s *bench.ReportContext) { s.Steps[1].Status = string(bench.Skipped) },
		func(_ *workload, s *bench.ReportContext) { s.Status = report.StatusFailed },
		func(w *workload, _ *bench.ReportContext) { w.benchmark.Phases[0].Streams[0].Complete = false },
		func(w *workload, _ *bench.ReportContext) { w.benchmark.Attempts[0].Status = "failed" },
		func(w *workload, _ *bench.ReportContext) { w.benchmark.FirstSet = 5 },
	} {
		w, snapshot = benchmarkReportFixture()
		change(w, &snapshot)
		contribution, _ = w.benchmarkContribution(snapshot)

		value = contribution.Data.(benchmarkState)
		if value.SequenceValid || value.Score != nil {
			t.Fatalf("invalid score: %+v", value)
		}
	}
}

func TestMaintenanceSQLContract(t *testing.T) {
	data, err := files.ReadFile("maintenance.sql")
	if err != nil {
		t.Fatal(err)
	}

	sql := bench.ParseSQL(string(data))

	for _, function := range maintenanceFunctions {
		if len(function.sources) == 0 {
			continue
		}

		if _, found := sql.Query(function.name, "rows"); !found {
			t.Fatal(function.name)
		}

		for _, source := range function.sources {
			ddl, found := sql.Query("staging", source)
			if !found || !strings.Contains(ddl, "CREATE TEMPORARY TABLE "+source) {
				t.Fatal(source)
			}
		}
	}
}

func TestMaintenanceDescendingDates(t *testing.T) {
	for _, table := range []string{"inventory", "store_sales", "catalog_sales", "web_sales"} {
		statements := maintenanceDeleteStatements(table, []string{"2002-07-25", "2002-07-18"})
		for _, statement := range statements {
			if !strings.Contains(statement, "BETWEEN '2002-07-18' AND '2002-07-25'") {
				t.Fatalf("descending delete range: %s", statement)
			}
		}
	}
}

func TestBenchmarkQueryWindow(t *testing.T) {
	start := time.Unix(100, 0)

	seconds, end := benchmarkQueryWindow([]benchmarkStream{
		{started: start, finished: start.Add(2 * time.Second)},
		{started: start.Add(time.Second), finished: start.Add(5 * time.Second)},
	})
	if seconds != 5 || !end.Equal(start.Add(5*time.Second)) {
		t.Fatalf("window %v %v", seconds, end)
	}
}
