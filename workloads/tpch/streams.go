package tpch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/mpp"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

type queryStream struct {
	ID         int                       `json:"id"`
	Seed       int64                     `json:"seed"`
	Order      [22]int                   `json:"order"`
	Parameters map[string]map[string]any `json:"parameters"`
	Attempted  int                       `json:"attempted"`
	Succeeded  int                       `json:"succeeded"`
	Errors     int                       `json:"errors"`
	Complete   bool                      `json:"complete"`
	started    time.Time
	finished   time.Time
}

type streamReport struct {
	Status             string        `json:"status"`
	Scope              string        `json:"scope"`
	TPCCompliant       bool          `json:"tpc_compliant"`
	Reason             string        `json:"reason"`
	Streams            []queryStream `json:"streams"`
	MeasurementSeconds float64       `json:"measurement_seconds"`
	QueriesPerHour     *float64      `json:"queries_per_hour,omitempty"`
}

func (w *workload) streamInitializer(dialect *mpp.Selection) bench.WorkerInit {
	check := dialect.Step(func(context.Context, *bench.Bench) error { return nil })

	return func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
		if err := check(ctx, b); err != nil {
			return nil, err
		}

		session, err := b.Session(ctx)
		if err != nil {
			return nil, err
		}

		return w.streamAction(session), nil
	}
}

func (w *workload) streamAction(session *bench.Session) bench.Action {
	return func(ctx context.Context, b *bench.Bench) error {
		stream := &w.streams[b.Worker()]
		b.Log.Warn("TPC-H query-only stream; RF1/RF2 refresh functions are not executed",
			"stream", stream.ID, "seed", stream.Seed)

		for _, number := range stream.Order {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			name := queryNames[number-1]
			body, _ := w.sql.Query(name, "body")

			start := time.Now()
			if stream.started.IsZero() {
				stream.started = start
			}

			_, err := session.RawRows(ctx, body, stream.Parameters[name])
			stream.finished = time.Now()
			elapsed := stream.finished.Sub(start).Seconds() * 1000
			w.recordAttempt(ctx, name, elapsed, err != nil)

			stream.Attempted++
			if err != nil {
				stream.Errors++

				if ctx.Err() != nil {
					return ctx.Err()
				}

				b.RecordError(name, err)

				continue
			}

			stream.Succeeded++
			b.Log.Info("query completed", "query", name, "stream", stream.ID,
				"seed", stream.Seed, "milliseconds", elapsed)
		}

		stream.Complete = stream.Attempted == len(queryNames)

		return nil
	}
}

func (w *workload) streamContribution(snapshot bench.ReportContext) (bench.ReportContribution, error) {
	if len(w.streams) == 0 {
		return bench.ReportContribution{
			Status: report.WorkloadReportSkipped, Reason: "generated query streams were not selected",
		}, nil
	}

	value := streamReport{
		Status: "complete", Scope: "query-only", Streams: w.streams,
		Reason: "RF1/RF2 refresh functions and full TPC-H benchmark sequencing are not executed",
	}
	if snapshot.Status != report.StatusCompleted {
		value.Status = "incomplete"
	}

	var first, last time.Time

	queries := 0

	for i := range w.streams {
		stream := &w.streams[i]
		if !stream.Complete || stream.Errors != 0 {
			value.Status = "incomplete"
		}

		if !stream.started.IsZero() && (first.IsZero() || stream.started.Before(first)) {
			first = stream.started
		}

		if stream.finished.After(last) {
			last = stream.finished
		}

		queries += stream.Succeeded
	}

	if !first.IsZero() && !last.IsZero() {
		value.MeasurementSeconds = last.Sub(first).Seconds()
	}

	if value.Status == "complete" && snapshot.Status == report.StatusCompleted && value.MeasurementSeconds > 0 {
		rate := float64(queries) * 3600 / value.MeasurementSeconds
		value.QueriesPerHour = &rate
	}

	return bench.ReportContribution{Data: value}, nil
}

func renderStreams(writer io.Writer, data json.RawMessage) error {
	var value streamReport
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	_, err := fmt.Fprintf(writer,
		"TPC-H query-only streams: %s; RF1/RF2 absent, not a full TPC-H result\n", value.Status)
	if err != nil {
		return err
	}

	if value.QueriesPerHour != nil {
		_, err := fmt.Fprintf(writer, "Query throughput: %.1f queries/hour (%d streams, %.6f seconds)\n",
			*value.QueriesPerHour, len(value.Streams), value.MeasurementSeconds)

		return err
	}

	return nil
}
