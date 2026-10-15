package compare

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func input(id, metrics string) Input {
	return Input{ID: id, Data: json.RawMessage(
		`{"kind":"run","schema":4,"workload":"custom","status":"completed",` +
			`"steps":[{"name":"workload","status":"completed"}],"metrics":` + metrics + `}`,
	)}
}

func TestComparisonPreservesExactNumbersScopesAndReference(t *testing.T) {
	a := input("a", `{"count":{"type":"counter","series":[
		{"attributes":{"step":"workload"},"total":9007199254740993},
		{"attributes":{"step":"warmup"},"total":4}]}}`)
	b := input("b", `{"count":{"type":"counter","series":[
		{"attributes":{"step":"workload"},"total":9007199254740994},
		{"attributes":{"step":"warmup"},"total":8}]}}`)

	result, err := Build([]Input{a, b}, "a")
	if err != nil {
		t.Fatal(err)
	}

	phases := 0

	for _, row := range result.Metrics {
		if row.Name != "count" {
			continue
		}

		phases++

		cell := row.Cells[1]
		if row.Attributes["step"] == "workload" && (string(*cell.Delta) != "1" || string(*cell.Value) != "9007199254740994") {
			t.Fatal("float rounding changed delta")
		}
	}

	if phases != 2 {
		t.Fatal("phases merged")
	}

	var output bytes.Buffer
	if err := WriteText(&output, result); err != nil || !strings.Contains(output.String(), "9007199254740994") {
		t.Fatal("human output lost source value")
	}

	if !bytes.Contains(result.Runs[0].Source, []byte("9007199254740993")) {
		t.Fatal("source changed")
	}
}

func TestComparisonMissingZeroAndIncompatible(t *testing.T) {
	for _, tc := range []struct{ name, first, second, reason string }{
		{"missing", `{"m":{"type":"gauge","unit":"s","total":1}}`, `{}`, "unavailable"},
		{"unit", `{"m":{"type":"gauge","unit":"s","total":1}}`, `{"m":{"type":"gauge","unit":"ms","total":1}}`, "unit"},
		{"type", `{"m":{"type":"gauge","total":1}}`, `{"m":{"type":"counter","total":1}}`, "type"},
		{"scope", `{"m":{"type":"gauge","total":1}}`, `{"m":{"type":"gauge","series":[{"total":1}]}}`, "dimensions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Build([]Input{input("a", tc.first), input("b", tc.second)}, "")
			if err != nil {
				t.Fatal(err)
			}

			found := false

			for _, row := range result.Metrics {
				if row.Cells[0].Value != nil {
					found = strings.Contains(row.Cells[1].Reason, tc.reason)
					if row.Cells[1].Delta != nil {
						t.Fatal("incompatible delta")
					}
				}
			}

			if !found {
				t.Fatalf("missing reason: %+v", result.Metrics)
			}
		})
	}

	result, err := Build([]Input{
		input("a", `{"m":{"type":"counter","total":0}}`),
		input("b", `{"m":{"type":"counter","total":10}}`),
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	cell := result.Metrics[0].Cells[1]
	if cell.Delta == nil || string(*cell.Delta) != "10" || cell.Percent != nil || cell.PercentReason != "zero reference" {
		t.Fatalf("zero-reference semantics: %+v", cell)
	}
}

func TestComparisonUnknownSchemaPayloadAndDifferences(t *testing.T) {
	a := input("a", `{"m":{"type":"gauge","total":1}}`)
	b := Input{ID: "b", Data: json.RawMessage(`{
		"kind":"run","schema":999,"workload":"other","status":"failed",
		"parameters":{"workload":{"seed":{"value":9007199254740993,"source":"cli"}}},
		"workload_reports":[{"kind":"unknown","schema":22,"data":{"opaque":true}}],
		"metrics":{"m":{"type":"gauge","total":3}}}`)}

	result, err := Build([]Input{a, b}, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Runs[1].Warnings) != 2 || result.Metrics[0].Cells[1].Value != nil || len(result.Differences) == 0 {
		t.Fatal("unknown/partial context hidden")
	}

	data, err := json.Marshal(result)
	if err != nil || !bytes.Contains(data, []byte("9007199254740993")) || !bytes.Contains(data, []byte("opaque")) {
		t.Fatal("unknown data discarded")
	}

	for _, inputs := range [][]Input{{a}, {a, a}} {
		if _, err := Build(inputs, ""); err == nil {
			t.Fatal("bad selection accepted")
		}
	}

	if _, err := Build([]Input{a, b}, "missing"); err == nil {
		t.Fatal("unselected reference accepted")
	}
}

func TestChangeDecimalPrecisionAndNegativeReference(t *testing.T) {
	for _, tc := range []struct {
		value, reference, delta, percent string
		rounded                          bool
	}{
		{"1.0000000000000001", "1", "0.0000000000000001", "0.00000000000001", false},
		{"2", "3", "-1", "-33.333333333333", true},
		{"-5", "-10", "5", "50", false},
	} {
		delta, percent, rounded := Change(json.Number(tc.value), json.Number(tc.reference))
		if delta == nil || percent == nil || string(*delta) != tc.delta ||
			string(*percent) != tc.percent || rounded != tc.rounded {
			t.Fatalf("unexpected decimal change for %+v: %v %v %t", tc, delta, percent, rounded)
		}
	}
}

func TestHistogramComparisonDoesNotAveragePercentiles(t *testing.T) {
	metrics := `{"latency":{"type":"histogram","unit":"ms","percentiles":{"p99":999},"series":[
		{"attributes":{"step":"warmup"},"count":10,"sum":20,"average":2,
		"bounds":[0,10,20],"bucket_counts":[0,10,0,0]},
		{"attributes":{"step":"workload"},"count":10,"sum":200,"average":20,
		"bounds":[0,10,20],"bucket_counts":[0,0,5,5]}]}}`

	result, err := Build([]Input{input("a", metrics), input("b", metrics)}, "")
	if err != nil {
		t.Fatal(err)
	}

	found := false

	for _, row := range result.Metrics {
		if row.Statistic == "p99" {
			t.Fatal("mixed-phase aggregate percentile assigned to series")
		}

		if row.Statistic == "bucket_p99" && row.Attributes["step"] == "workload" {
			found = true

			if row.Cells[0].Value != nil || !strings.Contains(row.Cells[0].Reason, "overflow") {
				t.Fatal("overflow quantile invented")
			}
		}

		if row.Statistic == "bucket_p50" && row.Attributes["step"] == "warmup" && string(*row.Cells[0].Value) != "5" {
			t.Fatal("bucket interpolation incorrect")
		}
	}

	if !found {
		t.Fatal("missing histogram diagnostics")
	}
}
