package compare

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestComparisonReferenceAndAggregateStepMismatch(t *testing.T) {
	first := input("first", `{"m":{"type":"counter","total":10}}`)
	second := input("second", `{"m":{"type":"counter","total":20}}`)

	result, err := Build([]Input{first, second}, "second")
	if err != nil {
		t.Fatal(err)
	}

	if string(*result.Metrics[0].Cells[0].Delta) != "-10" || string(*result.Metrics[0].Cells[0].Percent) != "-50" {
		t.Fatal("explicit reference ignored")
	}

	second.Data = json.RawMessage(strings.Replace(string(second.Data), `"metrics":`,
		`"step_selection":{"only":["warmup"]},"metrics":`, 1))

	result, err = Build([]Input{first, second}, "first")
	if err != nil {
		t.Fatal(err)
	}

	cell := result.Metrics[0].Cells[1]
	if cell.Value == nil || cell.Delta != nil || !strings.Contains(cell.DeltaReason, "scope") {
		t.Fatalf("aggregate scopes merged: %+v", cell)
	}
}

func TestComparisonHistogramBoundsAndTypedDimensions(t *testing.T) {
	first := input("first", `{"m":{"type":"histogram","unit":"ms","series":[
		{"attributes":{"step":"workload","tag":1},"count":2,"sum":10,"average":5,
		"bounds":[0,10],"bucket_counts":[0,2,0]}]}}`)
	second := input("second", `{"m":{"type":"histogram","unit":"ms","series":[
		{"attributes":{"step":"workload","tag":1},"count":2,"sum":10,"average":5,
		"bounds":[0,20],"bucket_counts":[0,2,0]},
		{"attributes":{"step":"workload","tag":"1"},"count":2,"sum":10,"average":5,
		"bounds":[0,10],"bucket_counts":[0,2,0]}]}}`)

	result, err := Build([]Input{first, second}, "")
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range result.Metrics {
		if strings.HasPrefix(row.Statistic, "bucket:") && row.Cells[0].Value != nil {
			if row.Cells[1].Value != nil || row.Cells[1].Delta != nil {
				t.Fatal("different boundaries or typed dimensions matched")
			}
		}
	}
}
