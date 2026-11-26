package tpch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/mpp"
)

var errBenchmarkMode = errors.New("TPC-H mode requires complete query streams, " +
	"writable retained sessions and a valid refresh range")

const benchmarkScope = "Unaudited execution and formula result; " +
	"not a certified TPC-H disclosure or price/performance result."

type benchmarkPhase struct {
	Name     string            `json:"name"`
	Streams  []benchmarkStream `json:"streams"`
	Refresh  []refreshAttempt  `json:"refresh"`
	Seconds  float64           `json:"measurement_seconds"`
	Complete bool              `json:"complete"`
}

type benchmarkStream struct {
	ID                int              `json:"id"`
	Seed              int64            `json:"seed"`
	Queries           []benchmarkQuery `json:"queries"`
	started, finished time.Time
}

type benchmarkQuery struct {
	Name    string  `json:"name"`
	Seconds float64 `json:"seconds"`
	Failed  bool    `json:"failed"`
	Rows    [][]any `json:"rows"`
}

type benchmarkRun struct {
	Number           int             `json:"number"`
	Power            *benchmarkPhase `json:"power,omitempty"`
	Throughput       *benchmarkPhase `json:"throughput,omitempty"`
	PowerAtSize      *float64        `json:"power_at_size,omitempty"`
	ThroughputAtSize *float64        `json:"throughput_at_size,omitempty"`
	QphHAtSize       *float64        `json:"qphh_at_size,omitempty"`
	composite        float64
}

type benchmarkReport struct {
	Mode            string     `json:"mode"`
	Status          string     `json:"status"`
	ScaleFactor     float64    `json:"scale_factor"`
	Streams         int        `json:"streams"`
	Seed            int64      `json:"seed"`
	SeedSource      string     `json:"seed_source"`
	LoadCompletedAt *time.Time `json:"load_completed_at,omitempty"`
	InvalidReasons  []string   `json:"invalid_reasons,omitempty"`
	Eligible        bool       `json:"eligible"`
	Backend         string     `json:"backend"`
	OwnSQL          bool       `json:"own_sql"`
	Unlogged        bool       `json:"unlogged"`
	loadComplete    bool
	loadCompleted   time.Time
	nextSet         int64
	FirstSet        int64           `json:"first_set"`
	NextSet         int64           `json:"next_set,omitempty"`
	Reason          string          `json:"reason"`
	Runs            []*benchmarkRun `json:"runs"`
	SelectedRun     int             `json:"selected_run,omitempty"`
	QphHAtSize      *float64        `json:"qphh_at_size,omitempty"`
}

type modeOptions struct {
	name            string
	streams         int
	streamsExplicit bool
	seed            int64
	seedExplicit    bool
	firstSet        int64
	setExplicit     bool
}

func selectMode(selection streamSelection) modeOptions {
	return modeOptions{
		name: selection.mode, streams: selection.count, streamsExplicit: selection.countExplicit, seed: selection.seed,
		seedExplicit: selection.seedExplicit, firstSet: selection.refreshSet, setExplicit: selection.refreshSetExplicit,
	}
}

func (w *workload) executeMode(d *bench.Def, dialect *mpp.Selection, options modeOptions) error {
	if !options.streamsExplicit {
		options.streams = max(1, minimumStreams(w.scaleFactor))
	}

	if err := w.validateMode(d, options); err != nil {
		return err
	}

	w.selectedMode = options.name

	return w.declareModes(d, dialect, options)
}

func (w *workload) declareModes(d *bench.Def, dialect *mpp.Selection, options modeOptions) error {
	g := d.Execution.Graph("tpch")

	var setup []bench.NodeRef
	for _, name := range []string{"drop_schema", "create_schema"} {
		setup = append(setup, g.Node(name, dialect.Step(sqlSection{w, name}.Run)))
	}

	setup = append(setup, g.Node("load_data", dialect.Step(w.load)))
	for _, name := range []string{"create_indexes", "analyze"} {
		action := sqlSection{w, name}.Run
		if name == "analyze" {
			action = func(ctx context.Context, b *bench.Bench) error {
				if err := (sqlSection{w, "analyze"}).Run(ctx, b); err != nil {
					return err
				}

				w.modeLoadCompleted = time.Now()

				return nil
			}
		}

		setup = append(setup, g.Node(name, dialect.Step(action)))
	}

	for index := 1; index < len(setup); index++ {
		setup[index].After(setup[index-1])
	}

	qualify := g.Node("validate_answers", dialect.Step(w.validate)).After(setup[len(setup)-1])
	g.Mode("qualification", "Qualification on unmodified data; no performance score",
		append(slices.Clone(setup), qualify)...)

	w.declarePerformanceModes(d, g, dialect, options, setup)

	return g.Run(options.name)
}

func (w *workload) declarePerformanceModes(
	d *bench.Def, g *bench.Graph, dialect *mpp.Selection, options modeOptions, setup []bench.NodeRef,
) {
	for _, name := range []string{"full", "power", "power-query-only", "throughput", "throughput-query-only"} {
		candidate := options
		candidate.name = name
		members := g.Group(name, func(group *bench.Graph) { w.declarePerformance(d, group, dialect, candidate, setup) })
		all := append(slices.Clone(setup), members...)
		g.Mode(name, "TPC-H "+name+" schedule", all...).Require(w.validateMode(d, candidate))
	}
}

func (w *workload) declarePerformance(
	d *bench.Def, g *bench.Graph, dialect *mpp.Selection, options modeOptions, setup []bench.NodeRef,
) {
	if w.validateMode(d, options) != nil {
		return
	}

	state := &benchmarkReport{
		Mode: options.name, ScaleFactor: w.scaleFactor, Streams: options.streams,
		Seed: options.seed, SeedSource: "explicit", FirstSet: options.firstSet, Reason: benchmarkScope,
		Backend: string(w.driverType), OwnSQL: w.sqlFile != "", Unlogged: w.useUnlogged, nextSet: options.firstSet,
	}
	if !options.seedExplicit {
		state.SeedSource = "load_completion"
	}

	if options.name == w.selectedMode {
		d.Report.Contribute("tpch.benchmark", 1, func(snapshot bench.ReportContext) (bench.ReportContribution, error) {
			return benchmarkContribution(state, snapshot), nil
		})
	}

	w.declarePerformanceNodes(g, dialect, options, setup, state)
}

func seedAtLoadCompletion(now time.Time) (int64, error) {
	value := int64(now.Month())*100000000 + int64(now.Day())*1000000 + int64(now.Hour())*10000 +
		int64(now.Minute())*100 + int64(now.Second())
	if value >= querySeedModulus {
		return 0, inputModeError("load completion seed exceeds query generator range")
	}

	return value, nil
}

func inputModeError(message string) error { return fmt.Errorf("%w: %s", errBenchmarkMode, message) }

func (w *workload) validateModeRefresh(d *bench.Def, options modeOptions) error {
	var pairs int

	switch options.name {
	case "power":
		pairs = 1
	case "throughput":
		pairs = options.streams
	case "full":
		pairs = 2 * (options.streams + 1)
	default:
		return nil
	}

	if w.useUnlogged || pairs < 1 || options.firstSet < 1 || int64(pairs) > lastExecutableRefreshSet-options.firstSet+1 {
		return errBenchmarkMode
	}

	if math.IsNaN(w.scaleFactor) || math.IsInf(w.scaleFactor, 0) || w.scaleFactor < 0.01 || w.scaleFactor > 100000 {
		return errRefreshScale
	}

	if !d.Execution.Enabled("load_data") && !options.setExplicit {
		return errRefreshExisting
	}

	if d.Execution.Enabled("load_data") && options.firstSet != 1 {
		return errRefreshFresh
	}

	return nil
}

func validateFullSelection(d *bench.Def) error {
	for _, name := range []string{
		"drop_schema", "create_schema", "load_data", "create_indexes", "analyze",
		"full/prepare_streams",
	} {
		if !d.Execution.Enabled(name) {
			return inputModeError("full mode cannot exclude required work: " + name)
		}
	}

	for number := 1; number <= 2; number++ {
		for _, phase := range []string{
			"power_rf1", "power_queries", "power_rf2",
			"throughput_queries", "throughput_refresh", "throughput_join",
		} {
			name := fmt.Sprintf("full/run_%d/%s", number, phase)
			if !d.Execution.Enabled(name) {
				return inputModeError("full mode cannot exclude required work: " + name)
			}
		}
	}

	return nil
}

func (w *workload) validateMode(d *bench.Def, options modeOptions) error {
	if options.name == "qualification" {
		return nil
	}

	if options.streams < 1 || int64(options.streams) >= querySeedModulus-options.seed {
		return errStreamSeed
	}

	if options.name == "full" && options.streams > int(lastExecutableRefreshSet/2)-1 {
		return errRefreshRange
	}

	if options.name == "full" {
		if minimumStreams(w.scaleFactor) == 0 || options.streams < minimumStreams(w.scaleFactor) || options.seedExplicit {
			return inputModeError("full mode needs an eligible scale, minimum streams, fresh load and load-derived seed")
		}

		if err := validateFullSelection(d); err != nil {
			return err
		}
	}

	if !options.seedExplicit && !d.Execution.Enabled("load_data") {
		return inputModeError("existing data needs an explicit query-seed")
	}

	if !streamDriverSupported(w.driverType) || options.streams < 1 {
		return errBenchmarkMode
	}

	if err := w.validateModeRefresh(d, options); err != nil {
		return err
	}

	for _, name := range queryNames {
		if _, ok := w.sql.Lookup(name, "body"); !ok {
			return fmt.Errorf("%w: missing %s", errStreamQuery, name)
		}
	}

	return nil
}
