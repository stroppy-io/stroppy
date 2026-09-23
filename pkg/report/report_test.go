package report

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRunJSONRoundTripKeepsUnknownWorkloadPayload(t *testing.T) {
	original := Run{
		Schema: SchemaVersion,
		Kind:   Kind,
		ID:     "report-1",
		Status: StatusCompleted,
		WorkloadReports: []WorkloadReport{{
			Kind: "custom.result", Schema: 7, Status: WorkloadReportOK,
			Data: json.RawMessage(`{"future":{"value":42}}`),
		}},
		Parameters: Parameters{Run: map[string]Parameter{}, Workload: map[string]Parameter{}},
		Metrics:    map[string]Metric{},
		Errors:     ErrorSummary{Groups: []ErrorGroup{}},
		Steps:      []Step{},
		Drivers:    []Driver{},
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Run
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Schema != SchemaVersion || decoded.Kind != Kind {
		t.Fatalf("identity = schema %d kind %q", decoded.Schema, decoded.Kind)
	}

	var payload map[string]any
	if err := json.Unmarshal(decoded.WorkloadReports[0].Data, &payload); err != nil {
		t.Fatal(err)
	}
	future := payload["future"].(map[string]any)
	if future["value"] != float64(42) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestOptionalFieldsStayAbsent(t *testing.T) {
	encoded, err := json.Marshal(Run{
		Schema: SchemaVersion, Kind: Kind, ID: "report-1", StartedAt: time.Unix(0, 0),
		FinishedAt: time.Unix(0, 0), Status: StatusCompleted,
		Parameters: Parameters{Run: map[string]Parameter{}, Workload: map[string]Parameter{}},
		Metrics:    map[string]Metric{}, Errors: ErrorSummary{Groups: []ErrorGroup{}},
		Steps: []Step{}, Drivers: []Driver{}, WorkloadReports: []WorkloadReport{},
	})
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}

	for _, absent := range []string{"run_id", "metadata", "custom", "failure"} {
		if _, ok := document[absent]; ok {
			t.Fatalf("optional field %q is present", absent)
		}
	}
}
