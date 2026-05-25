package insertprogress

import (
	"errors"
	"testing"
	"time"
)

func TestParseMode(t *testing.T) {
	t.Parallel()

	tests := map[string]Mode{
		"":        ModeBoth,
		"off":     ModeOff,
		"log":     ModeLog,
		"metrics": ModeMetrics,
		"both":    ModeBoth,
		" BOTH ":  ModeBoth,
	}

	for raw, expected := range tests {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			actual, err := ParseMode(raw)
			if err != nil {
				t.Fatalf("ParseMode(%q): %v", raw, err)
			}

			if actual != expected {
				t.Fatalf("ParseMode(%q) = %q, want %q", raw, actual, expected)
			}
		})
	}

	if _, err := ParseMode("console"); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("ParseMode(invalid) error = %v, want ErrInvalidMode", err)
	}
}

func TestTrackerFinalSnapshotPrefersGeneratedWhenConfirmedIsBehind(t *testing.T) {
	t.Parallel()

	var samples []Snapshot

	config := DefaultConfig()
	config.Mode = ModeMetrics
	config.Table = "stock"
	config.Method = "native"
	config.OnSample = func(snapshot Snapshot) {
		samples = append(samples, snapshot)
	}

	tracker := NewTracker(&config)
	tracker.SetTotal(100)
	tracker.SetWorkers(2)
	tracker.SetStage(0, StagePostgresCopyFrom)
	tracker.AddGenerated(0, 40)
	tracker.AddConfirmed(0, 10)

	snapshot := tracker.Finish(nil)

	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}

	if snapshot.Event != EventCompleted {
		t.Fatalf("event = %q, want %q", snapshot.Event, EventCompleted)
	}

	if snapshot.Rows != 40 || snapshot.RowKind != "generated" {
		t.Fatalf("rows = %d/%s, want 40/generated", snapshot.Rows, snapshot.RowKind)
	}

	if snapshot.InflightRows != 30 {
		t.Fatalf("inflight rows = %d, want 30", snapshot.InflightRows)
	}

	if snapshot.Percent != 40 {
		t.Fatalf("percent = %v, want 40", snapshot.Percent)
	}
}

func TestTrackerDetectsStall(t *testing.T) {
	t.Parallel()

	config := DefaultConfig()
	config.StallAfter = time.Second
	tracker := NewTracker(&config)
	tracker.SetTotal(10)
	tracker.lastProgress.Store(time.Now().Add(-2 * time.Second).UnixNano())

	snapshot := tracker.nextSnapshot(time.Now(), EventProgress)
	if !snapshot.Stalled {
		t.Fatal("snapshot.Stalled = false, want true")
	}

	if snapshot.Event != EventStall {
		t.Fatalf("event = %q, want %q", snapshot.Event, EventStall)
	}
}

func TestRowCounterFlushesBatchedRows(t *testing.T) {
	t.Parallel()

	config := DefaultConfig()
	tracker := NewTracker(&config)
	counter := RowCounter{tracker: tracker, flushEvery: 3}
	counter.Add(2)

	if got := tracker.generatedRows.Load(); got != 0 {
		t.Fatalf("generated after buffered add = %d, want 0", got)
	}

	counter.Add(1)

	if got := tracker.generatedRows.Load(); got != 3 {
		t.Fatalf("generated after threshold = %d, want 3", got)
	}

	counter.Add(2)
	counter.Flush()

	if got := tracker.generatedRows.Load(); got != 5 {
		t.Fatalf("generated after flush = %d, want 5", got)
	}
}
