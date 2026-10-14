// Package capture writes bounded run recordings independently of database work.
package capture

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

const (
	DefaultMaxBytes      = 64 << 20
	MaxRecordBytes       = 256 << 10
	queueRecords         = 256
	eventsMaxBytes       = 1 << 20
	captureDirectoryMode = 0o700
	captureFileMode      = 0o600
	metadataQueueRecords = 8
)

// Options selects independently optional streams and per-stream byte budgets.
type Options struct {
	Directory string
	Logs      bool
	Metrics   bool
	Interval  time.Duration
	MaxBytes  int64
	ReportID  string
	RunID     string
}

type (
	record struct{ data []byte }
	stream struct {
		file      *os.File
		stats     report.CaptureStream
		dropped   atomic.Uint64
		truncated atomic.Uint64
		stopped   atomic.Bool
		queue     chan record
	}
)

// Writer owns one fresh UUID directory. Producers never wait for disk writes.
type Writer struct {
	directory string
	start     time.Time
	manifest  report.Capture
	streams   map[string]*stream
	done      chan struct{}
	writers   sync.WaitGroup
	closeOnce sync.Once
	terminal  *report.Capture
	mu        sync.RWMutex
	closed    bool
	missed    atomic.Uint64
}

// New creates recording files exclusively before workload execution.
//
//nolint:gocognit // exclusive directory/file creation and rollback share one ownership boundary.
func New(options Options) (*Writer, error) {
	if options.Directory == "" {
		return nil, errDirectory
	}

	if options.MaxBytes == 0 {
		options.MaxBytes = DefaultMaxBytes
	}

	if options.MaxBytes < MaxRecordBytes || options.Interval <= 0 {
		return nil, errOptions
	}

	id := uuid.NewString()

	directory, err := filepath.Abs(filepath.Join(options.Directory, id))
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(options.Directory, captureDirectoryMode); err != nil {
		return nil, err
	}

	if err := os.Mkdir(directory, captureDirectoryMode); err != nil {
		return nil, err
	}

	w := &Writer{
		directory: directory, start: time.Now(), streams: map[string]*stream{},
		done: make(chan struct{}),
	}

	w.manifest = report.Capture{
		Schema: report.CaptureSchema, ID: id, Directory: directory, State: "recording",
		ReportID: options.ReportID, RunID: options.RunID,
		StartedAt: w.start.UTC(), IntervalSeconds: options.Interval.Seconds(), Streams: map[string]report.CaptureStream{},
	}
	for name, enabled := range map[string]bool{"logs": options.Logs, "metrics": options.Metrics, "events": true} {
		budget := options.MaxBytes
		if name == "events" {
			budget = eventsMaxBytes
		}

		capacity := queueRecords
		if name != "logs" {
			capacity = metadataQueueRecords
		}

		s := &stream{
			stats: report.CaptureStream{File: name + ".jsonl", Enabled: enabled, MaxBytes: budget},
			queue: make(chan record, capacity),
		}
		if !enabled {
			s.queue = nil
		}

		w.streams[name] = s

		if !enabled {
			continue
		}

		s.file, err = os.OpenFile(filepath.Join(directory, s.stats.File), os.O_CREATE|os.O_EXCL|os.O_WRONLY, captureFileMode)
		if err != nil {
			w.abort()

			return nil, err
		}
	}

	if err := w.writeManifest(); err != nil {
		w.abort()

		return nil, err
	}

	for _, s := range w.streams {
		if s.stats.Enabled {
			w.writers.Go(func() { w.write(s) })
		}
	}
	go func() { w.writers.Wait(); close(w.done) }()

	return w, nil
}

var (
	errDirectory = errors.New("capture requires a directory")
	errOptions   = errors.New("capture requires positive interval and at least 256 KiB per stream")
)

func (w *Writer) abort() {
	for _, s := range w.streams {
		if s.file != nil {
			_ = s.file.Close()
		}
	}

	_ = os.RemoveAll(w.directory)
}

func (w *Writer) Dropped(name string) {
	if s := w.streams[name]; s != nil && s.stats.Enabled {
		s.dropped.Add(1)
	}
}

func (w *Writer) ID() string       { return w.manifest.ID }
func (w *Writer) ReportID() string { return w.manifest.ReportID }

func (w *Writer) Enabled(name string) bool {
	s := w.streams[name]

	return s != nil && s.stats.Enabled && !s.stopped.Load()
}
func (w *Writer) Elapsed(now time.Time) float64 { return now.Sub(w.start).Seconds() }
func (w *Writer) Missed(count uint64)           { w.missed.Add(count) }

// Add encodes a record outside the file writer; oversize records are counted.
func (w *Writer) Add(name string, value any) {
	s := w.streams[name]
	if s == nil || !s.stats.Enabled {
		return
	}

	if s.stopped.Load() {
		s.dropped.Add(1)

		return
	}

	data, err := json.Marshal(value)
	if err != nil {
		s.dropped.Add(1)

		return
	}

	w.Enqueue(name, data)
}

// Enqueue copies bounded data and drops immediately when capture cannot keep up.
func (w *Writer) Enqueue(name string, data []byte) {
	s := w.streams[name]
	if s == nil || !s.stats.Enabled {
		return
	}

	if len(data) > MaxRecordBytes {
		s.truncated.Add(1)

		return
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed || s.stopped.Load() {
		s.dropped.Add(1)

		return
	}

	owned := append(append([]byte(nil), data...), '\n')
	select {
	case s.queue <- record{owned}:
	default:
		s.dropped.Add(1)
	}
}

func (w *Writer) Event(kind, step, state string, workers int, duration, drain time.Duration) {
	now := time.Now()
	w.Add("events", report.CaptureEvent{
		Time: now.UTC(), ElapsedSeconds: w.Elapsed(now), Kind: kind,
		Step: step, State: state, Workers: workers, DurationSeconds: duration.Seconds(), DrainSeconds: drain.Seconds(),
	})
}

func (w *Writer) write(s *stream) {
	for item := range s.queue {
		if s.stopped.Load() {
			s.dropped.Add(1)

			continue
		}

		if s.stats.Bytes+int64(len(item.data)) > s.stats.MaxBytes {
			s.truncated.Add(1)
			s.stopped.Store(true)

			continue
		}

		n, err := s.file.Write(item.data)

		s.stats.Bytes += int64(n)
		if err != nil {
			s.stats.Error = err.Error()
			s.stopped.Store(true)
			s.dropped.Add(1)

			continue
		}

		s.stats.Records++
	}

	if err := errors.Join(s.file.Sync(), s.file.Close()); err != nil {
		s.stats.Error = err.Error()
	}
}

// Close joins all writes and publishes terminal loss/error accounting.
func (w *Writer) Close() *report.Capture {
	w.closeOnce.Do(w.finish)
	result := *w.terminal

	result.Streams = make(map[string]report.CaptureStream, len(w.terminal.Streams))
	for name, stats := range w.terminal.Streams {
		result.Streams[name] = stats
	}

	return &result
}

func (w *Writer) finish() {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		for _, s := range w.streams {
			if s.stats.Enabled {
				close(s.queue)
			}
		}
	}
	w.mu.Unlock()
	<-w.done
	w.manifest.State = "complete"
	w.manifest.FinishedAt = time.Now().UTC()

	w.manifest.MissedSamples = w.missed.Load()
	for name, s := range w.streams {
		stats := s.stats
		stats.Dropped = s.dropped.Load()

		stats.Truncated = s.truncated.Load()
		if stats.Error != "" || stats.Dropped > 0 || stats.Truncated > 0 {
			w.manifest.State = "incomplete"
		}

		w.manifest.Streams[name] = stats
	}

	if w.manifest.MissedSamples > 0 {
		w.manifest.State = "incomplete"
	}

	if err := w.writeManifest(); err != nil {
		w.manifest.State = "failed"
		stats := w.manifest.Streams["events"]
		stats.Error = fmt.Sprintf("publish capture manifest: %v", err)
		w.manifest.Streams["events"] = stats
	}

	result := w.manifest
	w.terminal = &result
}

func (w *Writer) writeManifest() error {
	for name, s := range w.streams {
		if _, ok := w.manifest.Streams[name]; !ok {
			w.manifest.Streams[name] = s.stats
		}
	}

	data, err := json.MarshalIndent(w.manifest, "", "  ")
	if err != nil {
		return err
	}

	temporary := filepath.Join(w.directory, ".manifest.tmp")
	if err := os.WriteFile(temporary, append(data, '\n'), captureFileMode); err != nil {
		return err
	}

	return os.Rename(temporary, filepath.Join(w.directory, "manifest.json"))
}
