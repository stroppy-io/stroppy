package report

import (
	"encoding/json"
	"time"
)

const CaptureSchema = 1

// Capture identifies independently versioned append-only run artifacts.
type Capture struct {
	Schema          int                      `json:"schema"`
	ID              string                   `json:"id"`
	SourceID        string                   `json:"source_id,omitempty"`
	ReportID        string                   `json:"report_id,omitempty"`
	RunID           string                   `json:"run_id,omitempty"`
	Directory       string                   `json:"directory"`
	State           string                   `json:"state"`
	StartedAt       time.Time                `json:"started_at"`
	FinishedAt      time.Time                `json:"finished_at,omitempty"`
	IntervalSeconds float64                  `json:"interval_seconds"`
	MissedSamples   uint64                   `json:"missed_samples"`
	Streams         map[string]CaptureStream `json:"streams"`
	Owner           string                   `json:"owner,omitempty"`
}

// CaptureStream describes bounded recording and any loss of data.
type CaptureStream struct {
	File      string `json:"file"`
	Enabled   bool   `json:"enabled"`
	Records   uint64 `json:"records"`
	Bytes     int64  `json:"bytes"`
	MaxBytes  int64  `json:"max_bytes"`
	Dropped   uint64 `json:"dropped"`
	Truncated uint64 `json:"truncated"`
	Error     string `json:"error,omitempty"`
}

// MetricSample is a cumulative counter/histogram or recorded gauge snapshot.
type MetricSample struct {
	Sequence          uint64            `json:"sequence"`
	CaptureID         string            `json:"capture_id"`
	ReportID          string            `json:"report_id,omitempty"`
	Time              time.Time         `json:"time"`
	ElapsedSeconds    float64           `json:"elapsed_seconds"`
	CollectionSeconds float64           `json:"collection_seconds"`
	Semantics         string            `json:"semantics"`
	Metrics           map[string]Metric `json:"metrics"`
}

// CaptureEvent identifies execution boundaries using the capture clock.
type CaptureEvent struct {
	Time            time.Time `json:"time"`
	ElapsedSeconds  float64   `json:"elapsed_seconds"`
	Kind            string    `json:"kind"`
	Step            string    `json:"step,omitempty"`
	State           string    `json:"state,omitempty"`
	Workers         int       `json:"workers,omitempty"`
	DurationSeconds float64   `json:"duration_seconds,omitempty"`
	DrainSeconds    float64   `json:"drain_seconds,omitempty"`
}

// CapturedLog keeps structured fields alongside available execution correlation.
type CapturedLog struct {
	CaptureID      string          `json:"capture_id"`
	ReportID       string          `json:"report_id,omitempty"`
	Time           time.Time       `json:"time"`
	ElapsedSeconds float64         `json:"elapsed_seconds"`
	Level          string          `json:"level"`
	Logger         string          `json:"logger"`
	Message        string          `json:"message"`
	Fields         json.RawMessage `json:"fields"`
}
