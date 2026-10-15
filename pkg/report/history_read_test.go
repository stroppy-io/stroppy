package report

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func historyRun(workload string, started time.Time) *Run {
	return &Run{
		Schema: SchemaVersion, Kind: Kind, ID: "reused-run-id", StartedAt: started,
		FinishedAt: started.Add(time.Second), Workload: workload, Driver: "postgres", Status: StatusCompleted,
	}
}

func saveHistoryRun(t *testing.T, history *History, run *Run) string {
	t.Helper()

	path, err := history.Save(run)
	if err != nil {
		t.Fatal(err)
	}

	return strings.TrimSuffix(filepath.Base(path), ".json")
}

func TestHistoryQuery(t *testing.T) {
	t.Parallel()
	history := &History{Directory: t.TempDir()}
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	first := historyRun("tpcc/tx", start)
	first.Status = StatusFailed
	id1 := saveHistoryRun(t, history, first)
	second := historyRun("custom/test", start.Add(time.Hour))
	second.Driver = "noop"
	second.Drivers = []Driver{{Name: "default", Type: "noop"}, {Name: "primary", Type: "mysql"}}
	id2 := saveHistoryRun(t, history, second)
	id3 := saveHistoryRun(t, history, historyRun("tpcc/tx", start.Add(2*time.Hour)))

	id4 := saveHistoryRun(t, history, historyRun("tpcc/tx", start.Add(2*time.Hour)))
	for _, tc := range []struct {
		name  string
		query HistoryQuery
		ids   []string
	}{
		{"newest first and ID tie", HistoryQuery{}, []string{id4, id3, id2, id1}},
		{"oldest first", HistoryQuery{Reverse: true}, []string{id1, id2, id3, id4}},
		{"limit after filter", HistoryQuery{Workloads: []string{"tpcc/tx"}, Limit: 1}, []string{id4}},
		{"OR within field", HistoryQuery{Workloads: []string{"tpcc/tx", "custom/test"}}, []string{id4, id3, id2, id1}},
		{"AND across fields", HistoryQuery{Workloads: []string{"tpcc/tx"}, Statuses: []Status{StatusFailed}}, []string{id1}},
		{"named driver", HistoryQuery{Drivers: []string{"mysql"}}, []string{id2}},
		{"time bounds", HistoryQuery{Since: start.Add(time.Hour), Until: start.Add(2 * time.Hour)}, []string{id2}},
		{"empty", HistoryQuery{Workloads: []string{"missing"}}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			entries, problems, err := history.List(tc.query)
			if err != nil || len(problems) != 0 {
				t.Fatalf("List = %v, %v", problems, err)
			}

			ids := make([]string, 0, len(entries))
			for _, entry := range entries {
				ids = append(ids, entry.ID)
				if entry.ReportID != "reused-run-id" {
					t.Fatalf("report ID lost: %+v", entry)
				}
			}

			if !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("IDs = %v, want %v", ids, tc.ids)
			}
		})
	}

	for _, query := range []HistoryQuery{{Limit: -1}, {Since: start, Until: start}} {
		if _, _, err := history.List(query); err == nil {
			t.Fatal("invalid query accepted")
		}
	}
}

func TestHistoryPreservesUnknownJSONAndSkipsCorruption(t *testing.T) {
	t.Parallel()
	history := &History{Directory: t.TempDir()}
	id := saveHistoryRun(t, history, historyRun("future/workload", time.Now().UTC()))
	path := filepath.Join(history.Directory, id+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}

	document["schema"] = json.RawMessage(`99`)
	document["future_field"] = json.RawMessage(`{"large_integer":9007199254740993}`)
	document["workload_reports"] = json.RawMessage(`[{"kind":"unknown","schema":42,"data":{"new":true}}]`)

	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	badID := "2026-10-01T00-00-00Z-broken"
	if err := os.WriteFile(filepath.Join(history.Directory, badID+".json"), []byte(`{"kind":`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := history.Get(id)
	if err != nil || string(got) != string(data) {
		t.Fatalf("original JSON changed: %s, %v", got, err)
	}

	entries, problems, err := history.List(HistoryQuery{})
	if err != nil || len(entries) != 1 || len(problems) != 1 || problems[0].ID != badID {
		t.Fatalf("List = %v, %v, %v", entries, problems, err)
	}

	status, err := history.Status()
	if err != nil || status.Reports != 1 || status.Unreadable != 1 || status.SizeBytes <= int64(len(data)) {
		t.Fatalf("Status = %+v, %v", status, err)
	}

	if err := history.Remove(badID); err != nil {
		t.Fatalf("explicit corrupt removal: %v", err)
	}
}

func TestHistoryConfinesAccess(t *testing.T) {
	t.Parallel()
	history := &History{Directory: t.TempDir()}

	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	linkID := "2026-10-01T00-00-00Z-linked"
	if err := os.Symlink(outside, filepath.Join(history.Directory, linkID+".json")); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"../outside", outside, "", "2026-10-01T00-00-00Z-a/../../outside", linkID} {
		if _, err := history.Get(id); err == nil {
			t.Fatalf("Get accepted %q", id)
		}

		if err := history.Remove(id); err == nil {
			t.Fatalf("Remove accepted %q", id)
		}

		if err := history.CheckRemoval([]string{id}); err == nil {
			t.Fatalf("CheckRemoval accepted %q", id)
		}
	}

	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep" {
		t.Fatalf("outside file changed: %s, %v", data, err)
	}

	linkedDirectory := filepath.Join(t.TempDir(), "history")
	if err := os.Symlink(history.Directory, linkedDirectory); err != nil {
		t.Fatal(err)
	}

	if _, err := (&History{Directory: linkedDirectory}).Status(); err == nil {
		t.Fatal("symlink history accepted")
	}
}

func TestHistoryConcurrentPublication(t *testing.T) {
	t.Parallel()
	history := &History{Directory: t.TempDir()}
	run := historyRun("same/workload", time.Now().UTC())

	var wg sync.WaitGroup

	done := make(chan struct{})

	wg.Go(func() {
		defer close(done)

		for range 32 {
			wg.Go(func() {
				if _, err := history.Save(run); err != nil {
					t.Errorf("Save: %v", err)
				}
			})
		}
	})

	for {
		_, problems, err := history.List(HistoryQuery{})
		if err != nil || len(problems) != 0 {
			t.Fatalf("partial report visible: %v, %v", problems, err)
		}

		select {
		case <-done:
			wg.Wait()

			entries, _, err := history.List(HistoryQuery{})
			if err != nil || len(entries) != 32 {
				t.Fatalf("concurrent entries = %d, %v", len(entries), err)
			}

			for _, entry := range entries {
				info, err := os.Stat(filepath.Join(history.Directory, entry.ID+".json"))
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("report permissions: %v, %v", info, err)
				}
			}

			return
		default:
		}
	}
}

func TestHistoryTPSSelectsWorkloadOnly(t *testing.T) {
	t.Parallel()

	warmup, workload := 100.0, 42.0
	run := historyRun("tpcc/tx", time.Now().UTC())
	run.Metrics = map[string]Metric{"tps": {Total: &warmup, Series: []MetricSeries{
		{Attributes: map[string]any{"step": "warmup"}, Total: &warmup},
		{Attributes: map[string]any{"step": "workload"}, Total: &workload},
	}}}
	history := &History{Directory: t.TempDir()}
	saveHistoryRun(t, history, run)

	entries, _, err := history.List(HistoryQuery{})
	if err != nil || len(entries) != 1 || entries[0].TPS == nil || *entries[0].TPS != workload {
		t.Fatalf("TPS = %+v, %v", entries, err)
	}
}

func TestMissingHistoryIsEmptyWithoutCreatingStorage(t *testing.T) {
	t.Parallel()
	history := &History{Directory: filepath.Join(t.TempDir(), "not-created")}

	entries, problems, err := history.List(HistoryQuery{})
	if err != nil || len(entries) != 0 || len(problems) != 0 {
		t.Fatalf("List = %v, %v, %v", entries, problems, err)
	}

	status, err := history.Status()
	if err != nil || status.Exists || status.Reports != 0 {
		t.Fatalf("Status = %+v, %v", status, err)
	}

	if _, err := os.Stat(history.Directory); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("storage created: %v", err)
	}
}
