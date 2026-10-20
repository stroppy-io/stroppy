// Package view renders saved run reports without changing their data.
package view

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

const (
	maxTableColumns = 32
	maxTableCells   = 20_000
)

var errDocument = errors.New("expected a run report JSON object")

type field struct {
	Name  string
	Value node
}

type node struct {
	Title   string
	Text    string
	Fields  []field
	Items   []node
	Columns []string
	Rows    [][]node
	Summary string
}

type metricStat struct {
	Label string
	Text  string
	Exact string
	Unit  string
}

type metricView struct {
	Name   string
	Label  string
	Type   string
	Unit   string
	Stats  []metricStat
	Series int
	Data   node
	Charts []chart
}

type metricGroup struct {
	Name         string
	Distribution bool
	Metrics      []*metricView
}

type payloadView struct {
	Kind   string
	Schema string
	Status string
	Data   node
	Charts []chart
}

type page struct {
	Title        string
	ID           string
	Status       string
	Started      string
	Driver       string
	Warning      string
	Identity     node
	Steps        node
	Errors       node
	Failure      node
	Metrics      []metricView
	MetricGroups []metricGroup
	Payloads     []payloadView
	StepChart    chart
	Headlines    []field
	Original     string
	Download     string
	Catalog      string
	Capture      *captureView
}

func decode(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode report: %w", err)
	}

	if document == nil || text(document["kind"]) != report.Kind {
		return nil, errDocument
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errDocument
	}

	return document, nil
}

func reportPage(data []byte, id string) (page, error) {
	document, err := decode(data)
	if err != nil {
		return page{}, err
	}

	p := page{
		Title: text(document["workload"]), ID: id, Status: text(document["status"]),
		Started: text(document["started_at"]), Driver: text(document["driver"]), Original: string(data),
		Steps: tree(document["steps"]), Errors: tree(document["errors"]),
		StepChart: stepDurations(document["steps"]),
	}
	if p.ID == "" {
		p.ID = text(document["id"])
	}

	if p.Title == "" {
		p.Title = "Run report"
	}

	schema, _ := number(document["schema"])
	if schema < 1 || schema > float64(report.SchemaVersion) {
		p.Warning = "Unfamiliar report schema. Shared fields are shown where recognizable; " +
			"complete original data remains available."
	}

	identity := make(map[string]any)

	for key, value := range document {
		switch key {
		case "metrics", "steps", "errors", "workload_reports", "failure":
		default:
			identity[key] = value
		}
	}

	p.Identity = tree(identity)
	if failure := document["failure"]; failure != nil {
		p.Failure = tree(failure)
	}

	p.Metrics = metrics(document["metrics"])
	p.MetricGroups = groupMetrics(p.Metrics)

	p.Payloads = payloads(document["workload_reports"])
	for _, name := range []string{"tps", "queries_per_second", "iterations_per_second", "measurement_seconds"} {
		if value, ok := workloadValue(document["metrics"], name); ok {
			p.Headlines = append(p.Headlines, field{Name: headlineLabel(name), Value: node{Text: compact(value)}})
		}
	}

	return p, nil
}

func headlineLabel(name string) string {
	if name == "tps" {
		return "TPS"
	}

	return strings.ReplaceAll(name, "_", " ")
}

func tree(value any) node {
	switch v := value.(type) {
	case map[string]any:
		n := node{Summary: fmt.Sprintf("%d fields", len(v))}
		for _, key := range sortedKeys(v) {
			child := tree(v[key])
			if len(child.Fields) > 0 || len(child.Items) > 0 || len(child.Columns) > 0 {
				child.Title = child.Summary
			}

			n.Fields = append(n.Fields, field{Name: key, Value: child})
		}

		return n
	case []any:
		return arrayTree(v)
	default:
		return node{Text: text(value)}
	}
}

func arrayTree(items []any) node {
	n := node{Summary: fmt.Sprintf("%d entries", len(items))}
	if columns := arrayColumns(items); len(columns) > 0 {
		n.Columns = columns

		for _, item := range items {
			object, _ := item.(map[string]any)

			row := make([]node, 0, len(columns))
			for _, key := range columns {
				row = append(row, tree(object[key]))
			}

			n.Rows = append(n.Rows, row)
		}
	} else {
		for _, item := range items {
			n.Items = append(n.Items, tree(item))
		}
	}

	return n
}

func arrayColumns(items []any) []string {
	keys := map[string]any{}

	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil
		}

		for key := range object {
			keys[key] = nil
		}

		if len(keys) > maxTableColumns || len(keys) > maxTableCells/len(items) {
			return nil
		}
	}

	return sortedKeys(keys)
}

func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	for _, identity := range []string{"label", "query", "name"} {
		if index := slices.Index(keys, identity); index >= 0 {
			keys = append([]string{identity}, append(keys[:index], keys[index+1:]...)...)
		}
	}

	return keys
}

func text(value any) string {
	if value == nil {
		return "—"
	}

	return fmt.Sprint(value)
}

func number(value any) (float64, bool) {
	n, ok := value.(json.Number)
	if !ok {
		return 0, false
	}

	result, err := n.Float64()

	return result, err == nil
}

func compact(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func workloadValue(value any, name string) (float64, bool) {
	object, _ := value.(map[string]any)
	metric, _ := object[name].(map[string]any)

	series, _ := metric["series"].([]any)

	var workload float64

	matches := 0

	for _, item := range series {
		point, _ := item.(map[string]any)

		attributes, _ := point["attributes"].(map[string]any)
		if attributes["step"] == "workload" && len(attributes) == 1 {
			if recorded, valid := number(point["total"]); valid {
				workload = recorded
				matches++
			}
		}
	}

	if matches == 1 {
		return workload, true
	}

	return 0, false
}

func metrics(value any) []metricView {
	object, _ := value.(map[string]any)

	views := make([]metricView, 0, len(object))
	for _, name := range sortedKeys(object) {
		metric, ok := object[name].(map[string]any)
		if !ok {
			views = append(views, metricView{
				Name: name, Label: metricLabel(name), Type: "custom", Data: tree(object[name]),
				Stats: []metricStat{previewStat("Value", object[name], "")},
			})

			continue
		}

		m := metricView{
			Name: name, Label: metricLabel(name), Type: text(metric["type"]),
			Unit: metricUnit(metric["unit"]), Data: tree(metric),
		}
		m.Stats = metricStats(metric, m.Unit)

		series, _ := metric["series"].([]any)

		m.Series = len(series)
		if len(series) == 0 {
			m.Charts = appendHistogram(m.Charts, name, "aggregate · bucket boundaries in "+m.Unit, metric)
		}

		for _, item := range series {
			point, _ := item.(map[string]any)
			attributes, _ := point["attributes"].(map[string]any)

			scope := make([]string, 0, len(attributes)+1)

			scope = append(scope, "bucket boundaries in "+m.Unit)
			for _, key := range sortedKeys(attributes) {
				scope = append(scope, key+"="+text(attributes[key]))
			}

			m.Charts = appendHistogram(m.Charts, name, strings.Join(scope, ", "), point)
		}

		if len(m.Charts) == 0 {
			if plot, available := metricSeriesChart(name, m.Unit, series); available {
				m.Charts = append(m.Charts, plot)
			}
		}

		views = append(views, m)
	}

	return views
}

func payloads(value any) []payloadView {
	items, _ := value.([]any)

	views := make([]payloadView, 0, len(items))
	for i, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			views = append(views, payloadView{Kind: fmt.Sprintf("Result %d", i+1), Data: tree(item)})

			continue
		}

		p := payloadView{
			Kind: text(object["kind"]), Schema: text(object["schema"]),
			Status: text(object["status"]), Data: tree(object),
		}

		data, _ := object["data"].(map[string]any)
		for _, key := range sortedKeys(data) {
			if plot, available := numericObjectArray(key, data[key]); available {
				p.Charts = append(p.Charts, plot)
			}
		}

		for index := range p.Data.Fields {
			if p.Data.Fields[index].Name == "data" {
				p.Data.Fields[index].Value.Title = ""
			}
		}

		views = append(views, p)
	}

	return views
}
