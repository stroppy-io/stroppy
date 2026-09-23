// Package report defines Stroppy's versioned machine-readable run report.
package report

import (
	"encoding/json"
	"time"
)

const (
	// SchemaVersion is the current run report envelope version.
	SchemaVersion = 1
	// Kind identifies a benchmark run report among future report families.
	Kind = "run"
)

// Status describes how a run terminated.
type Status string

const (
	StatusCompleted           Status = "completed"
	StatusCompletedWithErrors Status = "completed_with_errors"
	StatusFailed              Status = "failed"
	StatusCanceled            Status = "canceled"
)

// WorkloadReportStatus describes one workload-owned report contribution.
type WorkloadReportStatus string

const (
	WorkloadReportOK      WorkloadReportStatus = "ok"
	WorkloadReportSkipped WorkloadReportStatus = "skipped"
	WorkloadReportError   WorkloadReportStatus = "error"
	WorkloadReportMissing WorkloadReportStatus = "missing"
)

// Run is one complete benchmark result. WorkloadReports remain opaque to the
// common envelope so new workload payloads do not require storage migrations.
type Run struct {
	Schema             int               `json:"schema"`
	Kind               string            `json:"kind"`
	ID                 string            `json:"id"`
	RunID              string            `json:"run_id,omitempty"`
	StroppyVersion     string            `json:"stroppy_version"`
	StartedAt          time.Time         `json:"started_at"`
	FinishedAt         time.Time         `json:"finished_at"`
	Status             Status            `json:"status"`
	Workload           string            `json:"workload"`
	Driver             string            `json:"driver"`
	Drivers            []Driver          `json:"drivers"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Host               Host              `json:"host"`
	Scenario           Scenario          `json:"scenario"`
	Parameters         Parameters        `json:"parameters"`
	StepSelection      StepSelection     `json:"step_selection"`
	Steps              []Step            `json:"steps"`
	MeasurementSeconds float64           `json:"measurement_seconds"`
	Metrics            map[string]Metric `json:"metrics"`
	Errors             ErrorSummary      `json:"errors"`
	Failure            *Failure          `json:"failure,omitempty"`
	WorkloadReports    []WorkloadReport  `json:"workload_reports"`
}

// Driver identifies one configured driver without persisting connection data.
type Driver struct {
	Index int    `json:"index"`
	Type  string `json:"type"`
}

// Host fingerprints the runtime environment used for a run.
type Host struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
	CPUs      int    `json:"cpus"`
	MaxProcs  int    `json:"gomaxprocs"`
}

// Scenario contains effective executor settings.
type Scenario struct {
	Executor            string   `json:"executor"`
	VUs                 int      `json:"vus"`
	DurationSeconds     *float64 `json:"duration_seconds"`
	IterationsRequested *int64   `json:"iterations_requested"`
}

// Parameters separates shared run inputs from workload-owned inputs.
type Parameters struct {
	Run      map[string]Parameter `json:"run"`
	Workload map[string]Parameter `json:"workload"`
}

// Parameter is one effective typed value and its winning input source.
type Parameter struct {
	Value  any    `json:"value"`
	Source string `json:"source"`
}

// StepSelection records explicit step filters.
type StepSelection struct {
	Only    []string `json:"only"`
	Exclude []string `json:"exclude"`
}

// Step is one step observed by the workload runtime.
type Step struct {
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
}

// Metric is a stable projection of one OpenTelemetry metric. Aggregate values
// stay convenient for scripts while Series preserves point dimensions.
type Metric struct {
	Type         string             `json:"type"`
	Unit         string             `json:"unit,omitempty"`
	Total        *float64           `json:"total,omitempty"`
	Count        *uint64            `json:"count,omitempty"`
	Sum          *float64           `json:"sum,omitempty"`
	Average      *float64           `json:"average,omitempty"`
	Bounds       []float64          `json:"bounds,omitempty"`
	BucketCounts []uint64           `json:"bucket_counts,omitempty"`
	Percentiles  map[string]float64 `json:"percentiles,omitempty"`
	Series       []MetricSeries     `json:"series,omitempty"`
}

// MetricSeries is one dimensioned counter, gauge, or histogram point.
type MetricSeries struct {
	Attributes   map[string]string `json:"attributes,omitempty"`
	Total        *float64          `json:"total,omitempty"`
	Count        *uint64           `json:"count,omitempty"`
	Sum          *float64          `json:"sum,omitempty"`
	Average      *float64          `json:"average,omitempty"`
	Bounds       []float64         `json:"bounds,omitempty"`
	BucketCounts []uint64          `json:"bucket_counts,omitempty"`
}

// ErrorSummary contains bounded nonfatal terminal error groups.
type ErrorSummary struct {
	TerminalErrors   uint64       `json:"terminal_errors"`
	FailedIterations uint64       `json:"failed_iterations"`
	FailedQueries    uint64       `json:"failed_queries"`
	RetryAttempts    uint64       `json:"retry_attempts"`
	Groups           []ErrorGroup `json:"groups"`
}

// ErrorGroup is one representative operation and driver-independent class.
type ErrorGroup struct {
	Operation string `json:"operation"`
	Class     string `json:"class"`
	Count     uint64 `json:"count"`
}

// Failure identifies the lifecycle phase that prevented clean completion.
type Failure struct {
	Phase  string `json:"phase"`
	Reason string `json:"reason"`
}

// WorkloadReport is a typed, independently versioned workload contribution.
type WorkloadReport struct {
	Kind   string               `json:"kind"`
	Schema int                  `json:"schema"`
	Status WorkloadReportStatus `json:"status"`
	Reason string               `json:"reason,omitempty"`
	Data   json.RawMessage      `json:"data,omitempty"`
}
