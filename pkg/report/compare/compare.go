// Package compare calculates explicit, dimension-preserving comparisons of saved runs.
package compare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"strings"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

const (
	Schema       = 1
	MaxRuns      = 8
	MinRuns      = 2
	percentScale = 100
)

var (
	ErrSelection = errors.New("comparison requires 2–8 distinct history IDs and a selected reference")
	errDocument  = errors.New("expected a single run report JSON document")
)

// Input retains the history identity separately from reusable run IDs.
type Input struct {
	ID   string
	Data json.RawMessage
}

// Run preserves source JSON, including unknown fields and exact source numbers.
type Run struct {
	ID       string          `json:"history_id"`
	Workload string          `json:"workload"`
	Status   string          `json:"status"`
	Source   json.RawMessage `json:"source"`
	Warnings []string        `json:"warnings"`
	document map[string]any
}

// Result is shared by machine, terminal, and HTML presentations.
type Result struct {
	Schema      int          `json:"schema"`
	Kind        string       `json:"kind"`
	Reference   string       `json:"reference"`
	Runs        []Run        `json:"runs"`
	Differences []Difference `json:"differences"`
	Metrics     []Row        `json:"metrics"`
	Warnings    []string     `json:"warnings"`
}

// Difference retains missing values separately from explicit nulls.
type Difference struct {
	Path   string  `json:"path"`
	Values []Value `json:"values"`
}

type Value struct {
	Present bool `json:"present"`
	Data    any  `json:"data"`
}

// Row is one statistic with an exact recorded type, unit, and dimension scope.
type Row struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Unit        string         `json:"unit"`
	Scope       string         `json:"scope"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	Statistic   string         `json:"statistic"`
	ValueUnit   string         `json:"value_unit"`
	Approximate bool           `json:"approximate,omitempty"`
	Bounds      []json.Number  `json:"bounds,omitempty"`
	Cells       []Cell         `json:"cells"`
}

// Cell distinguishes absent/incompatible data from a recorded zero.
type Cell struct {
	Value         *json.Number `json:"value"`
	Reason        string       `json:"reason,omitempty"`
	Delta         *json.Number `json:"delta"`
	Percent       *json.Number `json:"percent"`
	PercentReason string       `json:"percent_reason,omitempty"`
	Rounded       bool         `json:"rounded,omitempty"`
	DeltaReason   string       `json:"delta_reason,omitempty"`
}

// Build compares source documents without changing their recording semantics.
func Build(inputs []Input, reference string) (*Result, error) {
	if len(inputs) < 2 || len(inputs) > MaxRuns {
		return nil, ErrSelection
	}

	if reference == "" {
		reference = inputs[0].ID
	}

	result := &Result{
		Schema: Schema, Kind: "comparison", Reference: reference,
		Runs: []Run{}, Differences: []Difference{}, Metrics: []Row{},
		Warnings: []string{
			"Matching recorded settings do not establish equivalent database hardware, settings, or workload source.",
			"Deltas describe change, not improvement or statistical significance. Native TPS is not TPC-C tpmC.",
			"Empty recorded units are unspecified; no unit conversion or cross-dimension aggregation is performed.",
		},
	}
	seen := map[string]bool{}
	baseline := -1

	for i, input := range inputs {
		if input.ID == "" || seen[input.ID] {
			return nil, ErrSelection
		}

		seen[input.ID] = true
		if input.ID == reference {
			baseline = i
		}

		document, err := decode(input.Data)
		if err != nil {
			return nil, fmt.Errorf("report %s: %w", input.ID, err)
		}

		run := Run{
			ID: input.ID, Workload: stringValue(document["workload"]),
			Status: stringValue(document["status"]), Source: bytes.Clone(input.Data), document: document, Warnings: []string{},
		}
		if !knownSchema(document) {
			run.Warnings = append(run.Warnings, "Unknown report schema; numeric comparisons unavailable.")
		}

		if run.Status != string(report.StatusCompleted) {
			run.Warnings = append(run.Warnings, "Run is not cleanly completed; results may be partial or contain errors.")
		}

		result.Runs = append(result.Runs, run)
	}

	if baseline < 0 {
		return nil, ErrSelection
	}

	result.Differences = differences(result.Runs)
	result.Metrics = metricRows(result.Runs, baseline)

	return result, nil
}

func decode(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}

	if document == nil || stringValue(document["kind"]) != report.Kind {
		return nil, errDocument
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errDocument
	}

	return document, nil
}

func knownSchema(document map[string]any) bool {
	value, ok := document["schema"].(json.Number)
	if !ok {
		return false
	}

	n, err := value.Int64()

	return err == nil && n >= 1 && n <= report.SchemaVersion
}

func stringValue(value any) string {
	s, _ := value.(string)

	return s
}

func object(value any) map[string]any {
	m, _ := value.(map[string]any)

	return m
}

func items(value any) []any {
	a, _ := value.([]any)

	return a
}

func encoded(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "unavailable: " + err.Error()
	}

	return string(data)
}

func flatten(path string, value any, out map[string]Value) {
	if values, ok := value.(map[string]any); ok && len(values) > 0 {
		for key, child := range values {
			key = strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			flatten(path+"/"+key, child, out)
		}

		return
	}

	out[path] = Value{Present: true, Data: value}
}

func keyedEntries(value any, key string) any {
	entries, ok := value.([]any)
	if !ok {
		return value
	}

	result := map[string]any{}

	for _, item := range entries {
		name := stringValue(object(item)[key])
		if name == "" || result[name] != nil {
			return value
		}

		result[name] = item
	}

	return result
}

//nolint:gocognit // retain missing/null distinctions across arbitrary recorded context fields.
func differences(runs []Run) []Difference {
	contexts := make([]map[string]Value, len(runs))
	paths := map[string]bool{}

	for i, run := range runs {
		contexts[i] = map[string]Value{}

		for _, key := range []string{
			"schema", "workload", "driver", "drivers", "stroppy_version", "build_digest", "host",
			"parameters", "scenario", "step_selection", "executions", "measurements", "measurement_seconds",
			"status", "errors", "failure", "metadata", "custom", "workload_reports",
		} {
			value, present := run.document[key]
			if !present {
				continue
			}

			if key == "drivers" {
				value = keyedEntries(value, "name")
			}

			if key == "workload_reports" {
				value = keyedEntries(value, "kind")
			}

			flatten("/"+key, value, contexts[i])
		}

		for path := range contexts[i] {
			paths[path] = true
		}
	}

	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}

	slices.Sort(ordered)

	result := []Difference{}

	for _, path := range ordered {
		values := make([]Value, len(runs))
		for i := range runs {
			values[i] = contexts[i][path]
		}

		same := true

		for _, value := range values[1:] {
			if encoded(value) != encoded(values[0]) {
				same = false

				break
			}
		}

		if !same {
			result = append(result, Difference{Path: path, Values: values})
		}
	}

	return result
}

// Change calculates exact decimal subtraction and a bounded percentage representation.
func Change(value, reference json.Number) (delta, percent *json.Number, rounded bool) {
	v, ok := new(big.Rat).SetString(string(value))
	if !ok {
		return nil, nil, false
	}

	r, ok := new(big.Rat).SetString(string(reference))
	if !ok {
		return nil, nil, false
	}

	d := new(big.Rat).Sub(v, r)

	dn, dr := decimal(d)
	if r.Sign() == 0 {
		return &dn, nil, dr
	}

	p := new(big.Rat).Mul(new(big.Rat).Quo(d, new(big.Rat).Abs(r)), big.NewRat(percentScale, 1))
	pn, pr := decimal(p)

	return &dn, &pn, dr || pr
}

func decimal(value *big.Rat) (json.Number, bool) {
	places, exact := value.FloatPrec()
	if !exact {
		places = 12
	}

	text := value.FloatString(places)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}

	if text == "-0" {
		text = "0"
	}

	return json.Number(text), !exact
}
