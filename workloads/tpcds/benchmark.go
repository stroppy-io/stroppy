package tpcds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/dsdgen"
)

var (
	errBenchmarkOptions = errors.New("tpcds: benchmark needs pg/mysql, logged built-in schemas/queries, " +
		"an even streams count >=4, and default single-iteration executor settings")
	errBenchmarkFresh    = errors.New("tpcds: a fresh benchmark load requires refresh-set 1")
	errBenchmarkExisting = errors.New("tpcds: existing data requires explicit refresh-set from prior report")
)

const benchmarkDisclaimer = "Execution metric only; pricing, audit and data-accessibility " +
	"certification are not established."

type benchmarkQuery struct {
	Name    string  `json:"name"`
	Seconds float64 `json:"seconds"`
	Error   string  `json:"error,omitempty"`
}

type benchmarkStream struct {
	ID                int              `json:"id"`
	Queries           []benchmarkQuery `json:"queries"`
	Complete          bool             `json:"complete"`
	started, finished time.Time
}

type benchmarkPhase struct {
	Name     string            `json:"name"`
	Status   string            `json:"status"`
	Seconds  float64           `json:"seconds"`
	Streams  []benchmarkStream `json:"streams,omitempty"`
	finished time.Time
}

type benchmarkState struct {
	ScaleFactor   float64              `json:"scale_factor"`
	Sq            int                  `json:"sq"`
	FirstSet      int64                `json:"first_set"`
	NextSet       int64                `json:"next_set,omitempty"`
	Phases        []*benchmarkPhase    `json:"phases"`
	Attempts      []maintenanceAttempt `json:"maintenance"`
	SequenceValid bool                 `json:"sequence_valid"`
	Score         *float64             `json:"qphds_at_sf,omitempty"`
	TLD           float64              `json:"t_ld"`
	TPT           float64              `json:"t_pt"`
	TTT           float64              `json:"t_tt"`
	TDM           float64              `json:"t_dm"`
	TimeUnit      string               `json:"time_unit"`
	TPCCompliant  bool                 `json:"tpc_compliant"`
	Reason        string               `json:"reason"`
	directory     string
}

func benchmarkStreamDefault(enabled bool) int {
	if enabled {
		return 4
	}

	return 1
}

func benchmarkOptionsSupported(w *workload, settings bench.RunSettings) bool {
	return (w.driver == bench.DriverPostgres || w.driver == bench.DriverMySQL) && !w.useUnlogged &&
		w.schemaFile == "" && w.sqlFile == "" && w.genStream < 0 && w.streams >= 4 && w.streams%2 == 0 &&
		settings.Executor == "shared-iterations" && settings.Workers == 1 && settings.Iterations == 1 &&
		!math.IsNaN(w.scaleFactor) && !math.IsInf(w.scaleFactor, 0) && w.scaleFactor <= 100000
}

func (w *workload) configureBenchmark(d *bench.Def, settings bench.RunSettings, set int64, explicit bool) error {
	if !benchmarkOptionsSupported(w, settings) {
		return errBenchmarkOptions
	}

	if set > math.MaxInt64-int64(w.streams) {
		return errBenchmarkOptions
	}

	if d.Execution.Enabled("load_data") && set != 1 {
		return errBenchmarkFresh
	}

	if !d.Execution.Enabled("load_data") && !explicit {
		return errBenchmarkExisting
	}

	var err error

	w.maintenanceSQL, err = d.Queries.Load(files, "maintenance.sql")
	if err != nil {
		return err
	}

	w.benchmark = &benchmarkState{
		ScaleFactor: w.scaleFactor, Sq: w.streams, FirstSet: set, TimeUnit: "hours",
		Attempts: []maintenanceAttempt{}, Reason: benchmarkDisclaimer,
	}
	d.Report.Contribute("tpcds.benchmark", 1, w.benchmarkContribution)
	d.Report.Render("tpcds.benchmark", renderBenchmark)

	return nil
}

func (w *workload) executeBenchmark(d *bench.Def, wrap func(bench.Action) bench.Action) {
	d.Execution.Step("prepare_maintenance", wrap(w.initializeMaintenance))

	for _, phase := range []struct {
		name    string
		first   int
		workers int
	}{
		{"power", 0, 1},
		{"throughput_1", 1, w.streams},
		{"maintenance_1", 0, 0},
		{"throughput_2", w.streams + 1, w.streams},
		{"maintenance_2", 0, 0},
	} {
		state := &benchmarkPhase{Name: phase.name, Status: "not_started"}

		w.benchmark.Phases = append(w.benchmark.Phases, state)
		if phase.workers == 0 {
			w.executeMaintenancePhase(d, wrap, state)

			continue
		}

		state.Streams = make([]benchmarkStream, phase.workers)
		for worker := range phase.workers {
			state.Streams[worker].ID = phase.first + worker
		}

		result := d.Execution.WorkerStep(phase.name, w.benchmarkInitializer(state, wrap),
			bench.OncePerWorker(phase.workers))
		state.Status = string(result.Status)

		state.Seconds, state.finished = benchmarkQueryWindow(state.Streams)
		if phase.name == "throughput_2" && !state.finished.IsZero() {
			maintenanceEnd := w.benchmark.Phases[len(w.benchmark.Phases)-2].finished
			if !maintenanceEnd.IsZero() {
				state.Seconds = state.finished.Sub(maintenanceEnd).Seconds()
			}
		}
	}
}

func (w *workload) executeMaintenancePhase(
	d *bench.Def, wrap func(bench.Action) bench.Action, state *benchmarkPhase,
) {
	first := w.benchmark.FirstSet
	if state.Name == "maintenance_2" {
		first += int64(w.streams / 2)
	}

	result := d.Execution.Step(state.Name, wrap(func(ctx context.Context, b *bench.Bench) error {
		started := time.Now()
		err := w.runMaintenance(ctx, b, first)
		state.finished = time.Now()
		state.Seconds = state.finished.Sub(started).Seconds()

		return err
	}))
	state.Status = string(result.Status)
}

func (w *workload) benchmarkInitializer(
	phase *benchmarkPhase, wrap func(bench.Action) bench.Action,
) bench.WorkerInit {
	return func(ctx context.Context, b *bench.Bench) (bench.Action, error) {
		if err := wrap(func(context.Context, *bench.Bench) error { return nil })(ctx, b); err != nil {
			return nil, err
		}

		session, err := b.Session(ctx)
		if err != nil {
			return nil, err
		}

		stream := &phase.Streams[b.Worker()]

		queries, err := w.streamQueries(stream.ID)
		if err != nil {
			return nil, err
		}

		queries, err = benchmarkQueryOrder(queries, stream.ID)
		if err != nil {
			return nil, err
		}

		return func(ctx context.Context, _ *bench.Bench) error {
			return w.runBenchmarkStream(ctx, session, stream, queries)
		}, nil
	}
}

func (w *workload) runBenchmarkStream(
	ctx context.Context, session *bench.Session, stream *benchmarkStream, queries []namedQuery,
) error {
	stream.started = time.Now()
	defer func() { stream.finished = time.Now() }()

	for _, query := range queries {
		if err := ctx.Err(); err != nil {
			return err
		}

		started := time.Now()
		err := session.Exec(ctx, query.sql, nil)
		elapsed := time.Since(started)
		w.metrics[query.name].record(ctx, elapsed, err != nil)

		measurement := benchmarkQuery{Name: query.name, Seconds: elapsed.Seconds()}
		if err != nil {
			measurement.Error = err.Error()
		}

		stream.Queries = append(stream.Queries, measurement)

		if err != nil {
			return bench.Fatal(err)
		}
	}

	stream.Complete = true

	return nil
}

func benchmarkQueryWindow(streams []benchmarkStream) (float64, time.Time) {
	var first, last time.Time

	for i := range streams {
		stream := &streams[i]
		if !stream.started.IsZero() && (first.IsZero() || stream.started.Before(first)) {
			first = stream.started
		}

		if stream.finished.After(last) {
			last = stream.finished
		}
	}

	if first.IsZero() || last.IsZero() {
		return 0, last
	}

	return last.Sub(first).Seconds(), last
}

func (w *workload) generateMaintenance(ctx context.Context, _ *bench.Bench) error {
	directory, err := os.MkdirTemp("", "stroppy-tpcds-refresh-")
	if err != nil {
		return err
	}

	w.benchmark.directory = directory
	for set := w.benchmark.FirstSet; set < w.benchmark.FirstSet+int64(w.streams); set++ {
		path := filepath.Join(directory, strconv.FormatInt(set, 10))
		if err := os.Mkdir(path, 0o700); err != nil {
			return err
		}

		if err := dsdgen.GenerateRefresh(ctx, w.scaleFactor, set, path); err != nil {
			return err
		}
	}

	return nil
}

// Appendix D permutations reapply the same shuffle to the prior permutation.
func benchmarkQueryOrder(queries []namedQuery, stream int) ([]namedQuery, error) {
	permutation := make([]int, 99)
	for i := range permutation {
		permutation[i] = i
	}

	for range stream + 1 {
		random := dsdgen.NewRNStream(0, 0)
		for i := range permutation {
			j := dsdgen.GenerateUniformRandomInt(0, 98, random)
			permutation[i], permutation[j] = permutation[j], permutation[i]
		}
	}

	rank := map[int]int{}
	for i, id := range permutation {
		rank[id+1] = i
	}

	out := append([]namedQuery(nil), queries...)
	for _, query := range out {
		id, err := strconv.Atoi(strings.Split(strings.TrimPrefix(query.name, "query_"), "_")[0])
		if err != nil || id < 1 || id > 99 {
			return nil, errIncompleteQuerySet
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		left, _ := strconv.Atoi(strings.Split(strings.TrimPrefix(out[i].name, "query_"), "_")[0])

		right, _ := strconv.Atoi(strings.Split(strings.TrimPrefix(out[j].name, "query_"), "_")[0])
		if left == right {
			return out[i].name < out[j].name
		}

		return rank[left] < rank[right]
	})

	return out, nil
}

func benchmarkLoadSeconds(snapshot bench.ReportContext) (float64, bool) {
	wanted := map[string]bool{
		"create_schema": false, "load_data": false, "create_indexes": false,
		"analyze": false, "prepare_maintenance": false,
	}

	var seconds float64

	for _, step := range snapshot.Steps {
		if _, ok := wanted[step.Name]; ok {
			wanted[step.Name] = step.Status == string(bench.Completed)
			seconds += step.DurationSeconds
		}
	}

	for _, complete := range wanted {
		if !complete {
			return seconds, false
		}
	}

	return seconds, seconds > 0
}

func (w *workload) benchmarkContribution(snapshot bench.ReportContext) (bench.ReportContribution, error) {
	value := *w.benchmark
	value.TLD, value.TPT, value.TTT, value.TDM = 0, 0, 0, 0
	loadSeconds, loaded := benchmarkLoadSeconds(snapshot)
	value.SequenceValid = loaded && snapshot.Status == report.StatusCompleted && len(value.Phases) == 5 &&
		len(value.Attempts) == 11*w.streams && value.FirstSet == 1
	value.TLD = .01 * float64(w.streams) * loadSeconds / 3600

	value.Phases = make([]*benchmarkPhase, len(w.benchmark.Phases))
	for i, phase := range w.benchmark.Phases {
		copied := *phase
		copied.Streams = copyBenchmarkStreams(phase.Streams, snapshot)
		value.Phases[i] = &copied

		value.SequenceValid = value.SequenceValid && phase.Status == string(bench.Completed) && phase.Seconds > 0
		switch phase.Name {
		case "power":
			value.TPT = phase.Seconds * float64(w.streams) / 3600
		case "throughput_1", "throughput_2":
			value.TTT += phase.Seconds / 3600
		case "maintenance_1", "maintenance_2":
			value.TDM += phase.Seconds / 3600
		}

		for _, stream := range phase.Streams {
			value.SequenceValid = value.SequenceValid && stream.Complete && len(stream.Queries) == 103
		}
	}

	value.Attempts = append([]maintenanceAttempt(nil), value.Attempts...)
	for i := range value.Attempts {
		value.Attempts[i].Error = snapshot.Diagnostic(value.Attempts[i].Error)
		value.SequenceValid = value.SequenceValid && value.Attempts[i].Status == "completed"
	}

	value.Score = benchmarkScore(&value)
	if value.Score == nil {
		value.SequenceValid = false
		value.Reason = "Incomplete, failed or partial sequence; QphDS@SF is withheld."
	}

	if snapshot.Status != report.StatusCompleted {
		value.NextSet = 0
	}

	return bench.ReportContribution{Data: value}, nil
}

func copyBenchmarkStreams(streams []benchmarkStream, snapshot bench.ReportContext) []benchmarkStream {
	out := append([]benchmarkStream(nil), streams...)
	for i := range out {
		out[i].Queries = append([]benchmarkQuery(nil), streams[i].Queries...)
		for j := range out[i].Queries {
			out[i].Queries[j].Error = snapshot.Diagnostic(out[i].Queries[j].Error)
		}
	}

	return out
}

func benchmarkScore(value *benchmarkState) *float64 {
	if !value.SequenceValid || value.TLD <= 0 || value.TPT <= 0 || value.TTT <= 0 || value.TDM <= 0 {
		return nil
	}

	denominator := math.Pow(value.TLD*value.TPT*value.TTT*value.TDM, .25)

	score := math.Floor(value.ScaleFactor * (float64(value.Sq) * 99) / denominator)
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return nil
	}

	return &score
}

func renderBenchmark(writer io.Writer, data json.RawMessage) error {
	var value benchmarkState
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	if value.Score != nil {
		_, err := fmt.Fprintf(writer, "TPC-DS QphDS@%g: %.0f (Sq=%d; execution metric, not certified)\n",
			value.ScaleFactor, *value.Score, value.Sq)

		return err
	}

	_, err := fmt.Fprintf(writer, "TPC-DS QphDS@%g: unavailable (%s)\n", value.ScaleFactor, value.Reason)

	return err
}
