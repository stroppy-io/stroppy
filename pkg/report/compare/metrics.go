package compare

import (
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

type observation struct {
	row    Row
	value  *json.Number
	reason string
}

func rowKey(row *Row) string {
	return encoded([]any{row.Name, row.Type, row.Unit, row.Scope, row.Attributes, row.Statistic, row.Bounds})
}

func metricRows(runs []Run, reference int) []Row {
	observed := make([]map[string]observation, len(runs))
	all := map[string]bool{}

	for i, run := range runs {
		observed[i] = observations(&run)
		for key := range observed[i] {
			all[key] = true
		}
	}

	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	rows := make([]Row, 0, len(keys))
	for _, key := range keys {
		var row Row

		for _, values := range observed {
			if value, exists := values[key]; exists {
				row = value.row

				break
			}
		}

		row.Cells = make([]Cell, len(runs))
		for i, run := range runs {
			row.Cells[i] = comparisonValue(&run, &row, observed[i], key)
		}

		calculateChanges(&row, runs, reference)

		rows = append(rows, row)
	}

	return rows
}

func calculateChanges(row *Row, runs []Run, reference int) {
	base := row.Cells[reference].Value
	for i := range row.Cells {
		cell := &row.Cells[i]
		if cell.Value == nil {
			continue
		}

		if base == nil {
			cell.PercentReason = "reference unavailable"

			continue
		}

		if row.Scope == "aggregate-only" && !sameAggregateScope(&runs[reference], &runs[i]) {
			cell.DeltaReason = "aggregate measurement scope differs or is unknown"
			cell.PercentReason = cell.DeltaReason

			continue
		}

		cell.Delta, cell.Percent, cell.Rounded = Change(*cell.Value, *base)
		if cell.Percent == nil {
			cell.PercentReason = "zero reference"
		}
	}
}

func comparisonValue(run *Run, row *Row, observed map[string]observation, key string) Cell {
	if !knownSchema(run.document) {
		return Cell{Reason: "unsupported report schema"}
	}

	if sample, ok := observed[key]; ok {
		return Cell{Value: sample.value, Reason: sample.reason}
	}

	reason := "metric, statistic, or matching dimensions unavailable"

	if metric := object(object(run.document["metrics"])[row.Name]); metric != nil {
		switch {
		case stringValue(metric["type"]) != row.Type:
			reason = "incompatible metric type"
		case stringValue(metric["unit"]) != row.Unit:
			reason = "incompatible recorded unit"
		case len(row.Bounds) > 0:
			reason = "matching scope or histogram bounds unavailable"
		}
	}

	return Cell{Reason: reason}
}

func sameAggregateScope(first, second *Run) bool {
	if stringValue(first.document["workload"]) == "" || first.Workload != second.Workload ||
		len(items(first.document["steps"])) == 0 || len(items(second.document["steps"])) == 0 {
		return false
	}

	if encoded(first.document["step_selection"]) != encoded(second.document["step_selection"]) {
		return false
	}

	phases := func(run *Run) map[string]string {
		result := map[string]string{}

		for _, value := range items(run.document["steps"]) {
			step := object(value)
			result[stringValue(step["name"])] = stringValue(step["status"])
		}

		return result
	}

	return encoded(phases(first)) == encoded(phases(second))
}

func observations(run *Run) map[string]observation {
	result := map[string]observation{}

	for name, value := range object(run.document["metrics"]) {
		metric := object(value)
		typ, unit := stringValue(metric["type"]), stringValue(metric["unit"])

		series := items(metric["series"])
		if len(series) == 0 {
			addObservation(result, Row{Name: name, Type: typ, Unit: unit, Scope: "aggregate-only"}, metric)
		} else {
			for _, value := range series {
				point := object(value)

				attrs := object(point["attributes"])
				if len(attrs) == 0 {
					attrs = nil
				}

				row := Row{Name: name, Type: typ, Unit: unit, Scope: "series", Attributes: attrs}
				addObservation(result, row, point)
				// Aggregate percentiles share a scope only when there is one recorded series.
				if len(series) == 1 && typ == "histogram" {
					addPercentiles(result, row, metric)
				}
			}
		}
	}

	for _, item := range items(run.document["steps"]) {
		step := object(item)
		row := Row{
			Name: "step_duration", Type: "duration", Unit: "s", Scope: "step", Statistic: "duration_seconds",
			Attributes: map[string]any{"step": step["name"]},
		}

		reason := ""

		switch stringValue(step["status"]) {
		case "completed", "completed_with_errors", "failed", "canceled":
		default:
			reason = "step not executed"
		}

		putObservation(result, row, step["duration_seconds"], reason)
	}

	return result
}

//nolint:gocritic // each observation owns a row snapshot before its statistic changes.
func putObservation(result map[string]observation, row Row, value any, reason string) {
	row.ValueUnit = row.Unit
	if row.Type == "histogram" && (row.Statistic == "count" || strings.HasPrefix(row.Statistic, "bucket:")) {
		row.ValueUnit = "observations"
	}

	key := rowKey(&row)
	if _, duplicate := result[key]; duplicate {
		result[key] = observation{row: row, reason: "ambiguous duplicate series"}

		return
	}

	number, ok := value.(json.Number)
	if !ok && reason == "" {
		reason = "numeric statistic unavailable"
	}

	var recorded *json.Number
	if ok && reason == "" {
		recorded = &number
	}

	result[key] = observation{row: row, value: recorded, reason: reason}
}

//nolint:gocritic // statistic projection mutates an independent row copy.
func addObservation(result map[string]observation, row Row, point map[string]any) {
	switch row.Type {
	case "counter", "gauge":
		row.Statistic = "total"
		putObservation(result, row, point["total"], "")
	case "histogram":
		for _, statistic := range []string{"count", "sum", "average"} {
			row.Statistic = statistic

			reason := ""
			if statistic == "average" && fmt.Sprint(point["count"]) == "0" {
				reason = "no observations"
			}

			putObservation(result, row, point[statistic], reason)
		}

		addBuckets(result, row, point)
		addHistogramQuantiles(result, row, point)

		if row.Scope == "aggregate-only" {
			addPercentiles(result, row, point)
		}
	default:
		row.Statistic = "unsupported"
		putObservation(result, row, nil, "unsupported metric type")
	}
}

//nolint:gocritic // each statistic changes only its own row identity.
func addPercentiles(result map[string]observation, row Row, metric map[string]any) {
	row.Approximate = true
	for name, value := range object(metric["percentiles"]) {
		row.Statistic = name

		reason := ""
		if fmt.Sprint(metric["count"]) == "0" {
			reason = "no observations"
		}

		putObservation(result, row, value, reason)
	}
}

//nolint:gocognit,cyclop,gocritic // validate a copied histogram scope before estimating within finite buckets.
func addHistogramQuantiles(result map[string]observation, row Row, point map[string]any) {
	bounds, counts := items(point["bounds"]), items(point["bucket_counts"])
	if len(bounds) == 0 || len(counts) != len(bounds)+1 {
		return
	}

	previous := (*big.Rat)(nil)

	for _, bound := range bounds {
		value, valid := new(big.Rat).SetString(fmt.Sprint(bound))
		if !valid || (previous != nil && value.Cmp(previous) <= 0) {
			return
		}

		previous = value
	}

	total := new(big.Rat)
	for _, bucket := range counts {
		value, valid := new(big.Rat).SetString(fmt.Sprint(bucket))
		if !valid || !value.IsInt() || value.Sign() < 0 {
			return
		}

		total.Add(total, value)
	}

	count, ok := new(big.Rat).SetString(fmt.Sprint(point["count"]))
	if !ok || count.Sign() <= 0 || total.Cmp(count) != 0 {
		return
	}

	for _, percentile := range []int64{50, 90, 95, 99} {
		row.Statistic, row.Approximate = "bucket_p"+strconv.FormatInt(percentile, 10), true
		target := new(big.Rat).Mul(count, big.NewRat(percentile, percentScale))
		cumulative := new(big.Rat)
		reason := "histogram count inconsistent with buckets"

		var estimate any

		for i, bucket := range counts {
			n, valid := new(big.Rat).SetString(fmt.Sprint(bucket))
			if !valid || n.Sign() < 0 {
				break
			}

			before := new(big.Rat).Set(cumulative)
			cumulative.Add(cumulative, n)

			if cumulative.Cmp(target) < 0 {
				continue
			}

			if i == len(bounds) {
				reason = "quantile falls in unbounded overflow bucket"

				break
			}

			high, valid := new(big.Rat).SetString(fmt.Sprint(bounds[i]))
			if !valid || i == 0 {
				reason = "quantile falls in unbounded first bucket"

				break
			}

			low, valid := new(big.Rat).SetString(fmt.Sprint(bounds[i-1]))
			if !valid || high.Cmp(low) <= 0 || n.Sign() == 0 {
				break
			}

			fraction := new(big.Rat).Quo(new(big.Rat).Sub(target, before), n)
			value := new(big.Rat).Add(low, new(big.Rat).Mul(new(big.Rat).Sub(high, low), fraction))
			estimate, _ = decimal(value)
			reason = ""

			break
		}

		putObservation(result, row, estimate, reason)
	}
}

//nolint:gocritic // bucket identities must not mutate the parent histogram row.
func addBuckets(result map[string]observation, row Row, point map[string]any) {
	bounds, counts := items(point["bounds"]), items(point["bucket_counts"])
	if len(counts) != len(bounds)+1 {
		return
	}

	row.Bounds = make([]json.Number, 0, len(bounds))
	for _, bound := range bounds {
		n, ok := bound.(json.Number)
		if !ok {
			return
		}

		row.Bounds = append(row.Bounds, n)
	}

	for i, value := range counts {
		row.Statistic = "bucket:" + strconv.Itoa(i)
		putObservation(result, row, value, "")
	}
}

// Load resolves stable history IDs and retains input order for the default reference.
func Load(history *report.History, ids []string, reference string) (*Result, error) {
	if len(ids) < 2 || len(ids) > MaxRuns {
		return nil, ErrSelection
	}

	inputs := make([]Input, 0, len(ids))
	for _, id := range ids {
		data, err := history.Get(id)
		if err != nil {
			return nil, fmt.Errorf("report %s: %w", id, err)
		}

		inputs = append(inputs, Input{ID: id, Data: data})
	}

	return Build(inputs, reference)
}
