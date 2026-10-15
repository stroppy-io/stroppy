package view

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

var errCaptureFilter = errors.New("invalid capture filter")

func validateCaptureQuery(query url.Values) error {
	for _, key := range []string{"from", "to"} {
		if raw := query.Get(key); raw != "" {
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return errCaptureFilter
			}
		}
	}

	if raw := query.Get("log_page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 0 || page >= capturePreviewRecords/capturePageRecords {
			return errCaptureFilter
		}
	}

	if from, to := query.Get("from"), query.Get("to"); from != "" && to != "" {
		start, _ := strconv.ParseFloat(from, 64)

		end, _ := strconv.ParseFloat(to, 64)
		if end < start {
			return errCaptureFilter
		}
	}

	return nil
}

const (
	capturePageRecords    = 100
	capturePreviewRecords = 2000
	capturePreviewBytes   = 8 << 20
	captureRecordBytes    = 256 << 10
	maxTimelinePoints     = 240
	minimumTimelinePoints = 2
	captureSequenceBits   = 64
	timelineLeft          = 110.0
	timelineWidth         = 550.0
	timelineBottom        = 180.0
	timelineHeight        = 150.0
)

type captureView struct {
	Manifest       node
	State          string
	Timeline       []timeline
	Logs           []logView
	Events         node
	Previous       string
	Next           string
	Query          url.Values
	Notice         string
	MetricNames    []string
	ArtifactBase   string
	LogsEnabled    bool
	MetricsEnabled bool
	EventsEnabled  bool
	Complete       bool
}
type logView struct {
	Time    string
	Elapsed string
	Level   string
	Logger  string
	Message string
	Fields  node
}
type (
	timelinePoint struct {
		X, Y                float64
		Time, Value, Nearby string
		Sequence            uint64
	}
	timelineMarker struct {
		X     float64
		Label string
	}
	timeline struct {
		Name      string
		Scope     string
		Unit      string
		Semantics string
		Points    []timelinePoint
		Segments  []string
		Markers   []timelineMarker
		Maximum   string
		Minimum   string
		Elapsed   string
		Omitted   int
	}
)

func loadCapture(base string, c *report.Capture, query url.Values) captureView {
	result := captureView{State: report.CaptureState(base, c), Manifest: tree(captureMap(c)), Query: query}
	if c == nil {
		return result
	}

	result.Complete = result.State == "complete"
	result.LogsEnabled = c.Streams["logs"].Enabled
	result.MetricsEnabled = c.Streams["metrics"].Enabled
	result.EventsEnabled = c.Streams["events"].Enabled

	events, problem := captureRecords(base, c, "events.jsonl")
	result.Notice = problem

	result.Events = tree(events)
	if c.Streams["logs"].Enabled {
		result.Logs, result.Previous, result.Next, problem = captureLogs(base, c, query)
		result.Notice += " " + problem
	}

	if c.Streams["metrics"].Enabled {
		samples, metricProblem := captureRecords(base, c, "metrics.jsonl")
		result.Notice += " " + metricProblem
		result.Timeline, result.MetricNames = timelines(samples, events, query, c.IntervalSeconds)
	}

	result.Notice = strings.TrimSpace(result.Notice)

	return result
}

func captureMap(c *report.Capture) map[string]any {
	data, err := json.Marshal(c)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}

	var result map[string]any

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	_ = decoder.Decode(&result)

	return result
}

// walkCapture bounds every record and total bytes without retaining the stream.
//
//nolint:gocognit // distinguish bounded previews, corrupt records, and interrupted final lines.
func walkCapture(base string, c *report.Capture, name string, visit func(map[string]any) bool) string {
	file, err := report.OpenCaptureFile(base, c, name)
	if err != nil {
		return err.Error()
	}
	defer file.Close()

	reader := bufio.NewReaderSize(io.LimitReader(file, capturePreviewBytes+1), captureRecordBytes+1)
	consumed := 0

	for {
		line, err := reader.ReadSlice('\n')

		consumed += len(line)
		if consumed > capturePreviewBytes {
			return "Preview byte limit reached; complete stream remains downloadable."
		}

		if errors.Is(err, bufio.ErrBufferFull) {
			return "Oversized capture record omitted."
		}

		if len(line) > 0 {
			if line[len(line)-1] != '\n' {
				return "Incomplete final record omitted from " + name
			}

			var value map[string]any

			decoder := json.NewDecoder(strings.NewReader(string(line)))
			decoder.UseNumber()

			if decoder.Decode(&value) != nil {
				return "Malformed capture record in " + name
			}

			if !visit(value) {
				return "Preview record limit reached; complete stream remains downloadable."
			}
		}

		if err != nil {
			if !errors.Is(err, io.EOF) {
				return err.Error()
			}

			return ""
		}
	}
}

func captureRecords(base string, c *report.Capture, name string) (records []any, problem string) {
	records = []any{}
	problem = walkCapture(base, c, name, func(value map[string]any) bool {
		if len(records) >= capturePreviewRecords {
			return false
		}

		records = append(records, value)

		return true
	})

	return records, problem
}

//nolint:gocognit // log filtering and bounded pagination use one streaming callback.
func captureLogs(base string, c *report.Capture, query url.Values) (logs []logView, previous, next, problem string) {
	page, _ := strconv.Atoi(query.Get("log_page"))
	page = max(0, min(page, capturePreviewRecords/capturePageRecords-1))
	start := page * capturePageRecords
	from, _ := strconv.ParseFloat(query.Get("from"), 64)
	until, untilErr := strconv.ParseFloat(query.Get("to"), 64)
	logs = []logView{}
	matches := 0
	problem = walkCapture(base, c, "logs.jsonl", func(object map[string]any) bool {
		fields, _ := object["fields"].(map[string]any)
		if level := query.Get("level"); level != "" && object["level"] != level {
			return true
		}

		if step := query.Get("step"); step != "" && fields["step"] != step {
			return true
		}

		elapsed, _ := number(object["elapsed_seconds"])
		if elapsed < from || (untilErr == nil && elapsed > until) {
			return true
		}

		if search := strings.ToLower(query.Get("text")); search != "" {
			encoded, _ := json.Marshal(object)
			if !strings.Contains(strings.ToLower(string(encoded)), search) {
				return true
			}
		}

		matches++
		if matches <= start {
			return true
		}

		if len(logs) >= capturePageRecords {
			return false
		}

		logs = append(logs, logView{
			Time: text(object["time"]), Elapsed: compact(elapsed), Level: text(object["level"]),
			Logger: text(object["logger"]), Message: text(object["message"]), Fields: tree(fields),
		})

		return true
	})

	previous, next = "", ""
	if page > 0 {
		previous = capturePageLink(query, page-1)
	}

	if matches > start+len(logs) && page+1 < capturePreviewRecords/capturePageRecords {
		next = capturePageLink(query, page+1)
	}

	return logs, previous, next, problem
}

func copyCaptureQuery(values url.Values) url.Values {
	query := url.Values{}
	for key, value := range values {
		query[key] = value
	}

	return query
}

func nearbyCaptureLogs(values url.Values, elapsed, interval float64) string {
	query := copyCaptureQuery(values)
	query.Set("from", fmt.Sprintf("%.3f", max(0, elapsed-interval)))
	query.Set("to", fmt.Sprintf("%.3f", elapsed+interval))

	return capturePageLink(query, 0)
}

func capturePageLink(values url.Values, page int) string {
	query := copyCaptureQuery(values)
	query.Set("log_page", strconv.Itoa(page))

	return "?" + query.Encode() + "#captured-logs"
}

func selectedCaptureMetric(name, selected string) bool {
	if selected != "" {
		return name == selected
	}

	return name == "tps" || name == "queries_per_second" || name == "iterations_per_second"
}

//nolint:gocognit // project optional metric shapes without conflating series or units.
func timelines(samples, events []any, query url.Values, interval float64) (plots []timeline, metricNames []string) {
	selected := query.Get("metric")
	groups := map[string]*timeline{}
	names := map[string]any{}

	for _, value := range samples {
		sample, _ := value.(map[string]any)

		elapsed, sequence, valid := sampleClock(sample)
		if !valid {
			continue
		}

		metrics, _ := sample["metrics"].(map[string]any)
		for _, name := range sortedKeys(metrics) {
			names[name] = nil
			if !selectedCaptureMetric(name, selected) {
				continue
			}

			metric, _ := metrics[name].(map[string]any)

			points, _ := metric["series"].([]any)
			for _, item := range points {
				point, _ := item.(map[string]any)

				attrs, err := json.Marshal(point["attributes"])
				if err != nil {
					continue
				}

				for _, statistic := range []string{"total", "count", "average"} {
					recorded, ok := number(point[statistic])
					if !ok || math.IsNaN(recorded) || math.IsInf(recorded, 0) {
						continue
					}

					key := name + "\x00" + string(attrs) + "\x00" + statistic

					group := groups[key]
					if group == nil {
						unit := metricUnit(metric["unit"])
						if statistic == "count" {
							unit = "observations"
						}

						group = &timeline{
							Name: timelineName(name, statistic, metric["type"]), Unit: unit,
							Scope: string(attrs), Semantics: text(sample["semantics"]),
						}
						groups[key] = group
					}

					group.Points = append(group.Points, timelinePoint{
						X: elapsed, Y: recorded, Time: compact(elapsed), Value: text(point[statistic]),
						Sequence: sequence,
						Nearby:   nearbyCaptureLogs(query, elapsed, interval),
					})
				}
			}
		}
	}

	result := []timeline{}

	keys := map[string]any{}
	for key := range groups {
		keys[key] = nil
	}

	for _, key := range sortedKeys(keys) {
		group := groups[key]
		if len(group.Points) < minimumTimelinePoints {
			continue
		}

		projectTimeline(group, events, interval)
		result = append(result, *group)
	}

	metricNames = sortedKeys(names)
	if len(result) == 0 && selected == "" && len(metricNames) > 0 {
		query = copyCaptureQuery(query)
		query.Set("metric", metricNames[0])

		return timelines(samples, events, query, interval)
	}

	return result, metricNames
}

func sampleClock(sample map[string]any) (elapsed float64, sequence uint64, valid bool) {
	elapsed, elapsedOK := number(sample["elapsed_seconds"])

	serial, serialOK := number(sample["sequence"])
	if !elapsedOK || !serialOK || elapsed < 0 || serial < 0 || serial >= math.Exp2(captureSequenceBits) ||
		math.IsNaN(elapsed) || math.IsInf(elapsed, 0) || math.IsNaN(serial) || math.IsInf(serial, 0) {
		return 0, 0, false
	}

	return elapsed, uint64(serial), true
}

func timelineName(name, statistic string, metricType any) string {
	switch {
	case statistic == "count":
		return name + " · cumulative observations"
	case statistic == "average":
		return name + " · cumulative average"
	case metricType == "counter":
		return name + " · cumulative total"
	default:
		return name + " · recorded gauge"
	}
}

func projectTimeline(group *timeline, events []any, interval float64) {
	last := group.Points[len(group.Points)-1].X
	if last <= 0 {
		last = interval
	}

	low, high := 0.0, 0.0
	for _, point := range group.Points {
		low = min(low, point.Y)
		high = max(high, point.Y)
	}

	if high == low {
		high = low + 1
	}

	group.Minimum = previewNumber(json.Number(compact(low)))
	group.Maximum = previewNumber(json.Number(compact(high)))
	group.Elapsed = previewNumber(json.Number(compact(last)))
	stride := max(1, (len(group.Points)+maxTimelinePoints-1)/maxTimelinePoints)
	originalPoints := len(group.Points)
	projected := []timelinePoint{}
	segment := []string{}

	var previous *timelinePoint

	for i := range group.Points {
		point := group.Points[i]

		gap := previous != nil && (point.Sequence != previous.Sequence+1 || point.X-previous.X > interval*1.8)
		if gap && len(segment) > 0 {
			group.Segments = append(group.Segments, strings.Join(segment, " "))
			segment = nil
		}

		previous = &group.Points[i]
		if i%stride != 0 && i != len(group.Points)-1 {
			continue
		}

		point.X = timelineLeft + point.X/last*timelineWidth
		point.Y = timelineBottom - (point.Y-low)/(high-low)*timelineHeight
		segment = append(segment, fmt.Sprintf("%.2f,%.2f", point.X, point.Y))
		projected = append(projected, point)
	}

	if len(segment) > 0 {
		group.Segments = append(group.Segments, strings.Join(segment, " "))
	}

	group.Omitted = originalPoints - len(projected)

	group.Points = projected
	if group.Omitted > 0 {
		group.Segments = nil
	}

	for _, value := range events {
		event, _ := value.(map[string]any)

		elapsed, valid := number(event["elapsed_seconds"])
		if !valid || elapsed < 0 || math.IsNaN(elapsed) || math.IsInf(elapsed, 0) || elapsed > last {
			continue
		}

		group.Markers = append(group.Markers, timelineMarker{
			X:     timelineLeft + elapsed/last*timelineWidth,
			Label: strings.TrimSpace(text(event["kind"]) + " " + text(event["step"]) + " " + text(event["state"])),
		})
	}
}
