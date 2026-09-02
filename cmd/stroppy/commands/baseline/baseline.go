// Package baseline implements `stroppy baseline`: it measures stroppy's own
// overhead on the current machine — the noop-driver framework ceiling and the
// pg-wire protocol ceiling against a pg-noop blackhole server — and saves a
// versioned report for future comparison.
package baseline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/stroppy-io/stroppy/internal/pgnoop"
	"github.com/stroppy-io/stroppy/internal/version"
	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/pkg/common/logger"
	"github.com/stroppy-io/stroppy/pkg/config"
)

const (
	workloadName = "baseline"

	tierNoop = "noop"
	tierWire = "wire"

	defaultDuration = 3 * time.Second
	quickDuration   = 1 * time.Second

	defaultRows = 250_000
	quickRows   = 100_000

	envServerPath = "STROPPY_PG_NOOP_PATH"
)

var (
	errUnknownTier       = errors.New("unknown tier; expected noop or wire")
	errNoTiers           = errors.New("no tiers selected")
	errUnknownDownload   = errors.New("unknown --download value; expected ask, always, or never")
	errMarshalReportFail = errors.New("marshal report")
	errFreePortNotTCP    = errors.New("free port listener address is not TCP")
	errVUsOutOfRange     = errors.New("vus out of range")
)

type options struct {
	quick      bool
	jsonOut    bool
	tiers      []string
	vus        int
	duration   time.Duration
	rows       int64
	serverPath string
	download   string
	noSave     bool
}

var opts options

// Cmd is the `stroppy baseline` subcommand.
var Cmd = &cobra.Command{
	Use:   "baseline",
	Short: "Measure stroppy's own performance on this machine",
	Long: `Measure the stroppy ceiling on this machine: no database required.

Two tiers run the built-in baseline workload back to back:

  noop  the noop driver — pure framework cost: row generation, argument
        handling, transaction bookkeeping.
  wire  the postgres driver against a pg-noop blackhole server on loopback —
        framework + pgx pool + PostgreSQL wire protocol. pg-noop discards all
        I/O, so this tier isolates client-side cost: the ceiling a real
        database run can never exceed.

Each tier measures load throughput (rows/s) and transaction iterations at one
VU and at GOMAXPROCS VUs. Verdicts check hardware-independent invariants
(parallel scaling, loopback latency floor, measurement sanity) instead of
absolute thresholds, and a versioned JSON report is saved under
~/.stroppy/baselines/ as ground for future comparisons.

The pg-noop server binary is resolved from an embedded copy (release builds),
the ~/.stroppy/bin/ cache, or the pinned GitHub release (downloaded with
consent). Use --server-path or STROPPY_PG_NOOP_PATH to supply it directly.`,
	Example: `
  stroppy baseline                     # full two-tier run, ~20s
  stroppy baseline --quick             # 1s phases, smaller load
  stroppy baseline --tiers noop        # framework tier only, no server needed
  stroppy baseline --json              # machine-readable report on stdout
  stroppy baseline --server-path ./pgnoop
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return run(cmd.Context(), cmd.OutOrStdout())
	},
}

func init() {
	Cmd.Flags().BoolVar(&opts.quick, "quick", false, "shorter phases and a smaller load")
	Cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "print the report as JSON")
	Cmd.Flags().StringSliceVar(&opts.tiers, "tiers", []string{tierNoop, tierWire},
		"tiers to run: noop, wire")
	Cmd.Flags().IntVar(&opts.vus, "vus", runtime.GOMAXPROCS(0),
		"VU count for the parallel tx phase")
	Cmd.Flags().DurationVar(&opts.duration, "duration", 0,
		"tx phase duration (default 3s, 1s with --quick)")
	Cmd.Flags().Int64Var(&opts.rows, "rows", 0,
		"load rows (default 250000, 100000 with --quick)")
	Cmd.Flags().StringVar(&opts.serverPath, "server-path", "",
		"path to a pg-noop binary (env STROPPY_PG_NOOP_PATH)")
	Cmd.Flags().StringVar(&opts.download, "download", "ask",
		"server download consent: ask, always, or never")
	Cmd.Flags().BoolVar(&opts.noSave, "no-save", false,
		"do not write the report to ~/.stroppy/baselines/")
}

// runPlan is the validated shape of one baseline invocation.
type runPlan struct {
	tiers    []string
	consent  pgnoop.Consent
	duration time.Duration
	rows     int64
	vus      int
}

func planRun() (runPlan, error) {
	tiers, err := parseTiers(opts.tiers)
	if err != nil {
		return runPlan{}, err
	}

	consent, err := consentFrom(opts.download)
	if err != nil {
		return runPlan{}, err
	}

	duration := opts.duration
	if duration <= 0 {
		duration = defaultDuration
	}

	rows := opts.rows
	if rows <= 0 {
		rows = defaultRows
	}

	if opts.quick {
		if opts.duration <= 0 {
			duration = quickDuration
		}

		if opts.rows <= 0 {
			rows = quickRows
		}
	}

	// VU counts convert to int32 pool sizing; reject values that would wrap.
	if opts.vus < 1 || opts.vus > math.MaxInt32 {
		return runPlan{}, fmt.Errorf("%w: got %d, want 1..%d", errVUsOutOfRange, opts.vus, math.MaxInt32)
	}

	return runPlan{
		tiers:    tiers,
		consent:  consent,
		duration: duration,
		rows:     rows,
		vus:      opts.vus,
	}, nil
}

func run(ctx context.Context, out io.Writer) error {
	// Warn level keeps phase logs off stderr; the command renders its own report.
	if err := logger.Init("warn", "development"); err != nil {
		return err
	}

	plan, err := planRun()
	if err != nil {
		return err
	}

	report := Report{
		Schema:  reportSchema,
		Stroppy: version.Version,
		Time:    time.Now().UTC(),
		Host:    hostInfo(),
	}

	if err := measureTiers(ctx, plan, &report); err != nil {
		return err
	}

	report.Verdicts = evaluate(report.Tiers)

	return emitReport(out, &report)
}

func measureTiers(ctx context.Context, plan runPlan, report *Report) error {
	var serverBinary string

	if slices.Contains(plan.tiers, tierWire) {
		resolved, err := pgnoop.Resolve(pgnoop.Options{
			Path:    serverPathOrEnv(),
			Consent: plan.consent,
			Log: func(format string, args ...any) {
				fmt.Fprintf(os.Stderr, format+"\n", args...)
			},
		})
		if err != nil {
			return err
		}

		serverBinary = resolved
		report.PGNoop = pgnoop.Version
	}

	for _, name := range plan.tiers {
		var (
			tier TierResult
			err  error
		)

		switch name {
		case tierNoop:
			tier, err = measureTier(ctx, tierNoop, noopDriver(), plan)
		case tierWire:
			tier, err = measureWireTier(ctx, serverBinary, plan)
		}

		if err != nil {
			return err
		}

		report.Tiers = append(report.Tiers, tier)
	}

	return nil
}

func emitReport(out io.Writer, report *Report) error {
	previous, _ := loadPrevious(report.Time) //nolint:errcheck // a missing history is not a failure

	if opts.jsonOut {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: %w", errMarshalReportFail, err)
		}

		fmt.Fprintln(out, string(data))
	} else {
		renderText(out, report)
	}

	if opts.noSave {
		return nil
	}

	path, err := saveReport(report)
	if err != nil {
		return err
	}

	// stdout stays a valid JSON document in --json mode; status goes to stderr.
	statusOut := out
	if opts.jsonOut {
		statusOut = os.Stderr
	}

	fmt.Fprintf(statusOut, "\nhistory: saved %s\n", path)

	if previous != nil {
		renderDiff(statusOut, previous, report)
	}

	return nil
}

func measureWireTier(ctx context.Context, binary string, plan runPlan) (TierResult, error) {
	port, err := freePort(ctx)
	if err != nil {
		return TierResult{}, err
	}

	server, err := pgnoop.Start(ctx, binary, port)
	if err != nil {
		return TierResult{}, err
	}
	defer func() { _ = server.Stop() }()

	fmt.Fprintf(os.Stderr, "pg-noop %s ready on %s\n", pgnoop.Version, server.Addr())

	return measureTier(ctx, tierWire, wireDriver(server.Addr(), plan.vus), plan)
}

func serverPathOrEnv() string {
	if opts.serverPath != "" {
		return opts.serverPath
	}

	return os.Getenv(envServerPath)
}

func parseTiers(names []string) ([]string, error) {
	var tiers []string

	for _, raw := range names {
		name := strings.TrimSpace(strings.ToLower(raw))
		if name != tierNoop && name != tierWire {
			return nil, fmt.Errorf("%w: %q", errUnknownTier, raw)
		}

		tiers = append(tiers, name)
	}

	if len(tiers) == 0 {
		return nil, errNoTiers
	}

	return tiers, nil
}

func runPhase(
	ctx context.Context,
	driver *config.DriverConfig,
	steps []string,
	cli map[string]string,
) (runMetrics, error) {
	var captured runMetrics

	metrics := &bench.MetricsConfig{
		Quiet: true,
		OnSummary: func(data metricdata.ResourceMetrics) {
			captured = extractMetrics(data)
		},
	}

	err := bench.Run(
		ctx,
		workloadName,
		map[int]*config.DriverConfig{0: driver},
		bench.ParamInputs{CLI: cli},
		steps,
		nil,
		logger.Global(),
		metrics,
	)

	return captured, err
}

func loadPhaseCLI(rows, workers int64) map[string]string {
	return map[string]string{
		"executor":     "shared-iterations",
		"vus":          "1",
		"iterations":   "1",
		"rows":         strconv.FormatInt(rows, 10),
		"load-workers": strconv.FormatInt(workers, 10),
	}
}

func txPhaseCLI(vus int, duration time.Duration) map[string]string {
	return map[string]string{
		"executor": "constant-vus",
		"vus":      strconv.Itoa(vus),
		"duration": duration.String(),
	}
}

func txStat(m *runMetrics, duration time.Duration) TxStat {
	stat := TxStat{
		Iterations: m.iterations,
		Failed:     m.failed,
		AvgMs:      m.iter.avgMs(),
		P50Ms:      m.iter.quantile(medianQ),
		P90Ms:      m.iter.quantile(p90Q),
		P99Ms:      m.iter.quantile(p99Q),
	}

	if duration > 0 {
		stat.TxPerSec = m.iterations / duration.Seconds()
	}

	return stat
}

func loadStat(m *runMetrics) LoadStat {
	stat := LoadStat{Rows: m.insertRows, DurationMs: m.insertDurMs}
	if m.insertDurMs > 0 {
		stat.RowsPerSec = m.insertRows / (m.insertDurMs / msPerSecond)
	}

	return stat
}

func measureTier(
	ctx context.Context,
	name string,
	driver *config.DriverConfig,
	plan runPlan,
) (TierResult, error) {
	tier := TierResult{Name: name, ParallelVUs: plan.vus}

	load, err := runPhase(ctx, driver,
		[]string{"drop_schema", "create_schema", "load_data"},
		loadPhaseCLI(plan.rows, int64(runtime.GOMAXPROCS(0))))
	if err != nil {
		return tier, fmt.Errorf("%s load phase: %w", name, err)
	}

	tier.Load = loadStat(&load)

	single, err := runPhase(ctx, driver, []string{"workload"}, txPhaseCLI(1, plan.duration))
	if err != nil {
		return tier, fmt.Errorf("%s tx phase (1 VU): %w", name, err)
	}

	tier.TxSingle = txStat(&single, plan.duration)

	parallel, err := runPhase(ctx, driver, []string{"workload"}, txPhaseCLI(plan.vus, plan.duration))
	if err != nil {
		return tier, fmt.Errorf("%s tx phase (%d VUs): %w", name, plan.vus, err)
	}

	tier.TxParallel = txStat(&parallel, plan.duration)

	return tier, nil
}

func noopDriver() *config.DriverConfig {
	return &config.DriverConfig{DriverType: config.DriverTypeNoop}
}

func wireDriver(addr string, vus int) *config.DriverConfig {
	maxConns := int32(max(vus, 1)) //nolint:gosec // G115: VU counts stay in int32 range

	return &config.DriverConfig{
		DriverType: config.DriverTypePostgres,
		URL:        fmt.Sprintf("postgres://stroppy@%s/postgres?sslmode=disable", addr),
		Postgres:   &config.PostgresConfig{MaxConns: &maxConns},
	}
}

func freePort(ctx context.Context) (int, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find free port: %w", err)
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errFreePortNotTCP
	}

	return addr.Port, nil
}

func consentFrom(name string) (pgnoop.Consent, error) {
	switch strings.ToLower(name) {
	case "ask":
		return pgnoop.ConsentAsk, nil
	case "always":
		return pgnoop.ConsentAlways, nil
	case "never":
		return pgnoop.ConsentNever, nil
	default:
		return pgnoop.ConsentAsk, fmt.Errorf("%w: %q", errUnknownDownload, name)
	}
}

func hostInfo() HostInfo {
	return HostInfo{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
		CPUs:      runtime.NumCPU(),
		MaxProcs:  runtime.GOMAXPROCS(0),
	}
}
