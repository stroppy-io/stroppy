package baseline

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

const (
	reportSchema = 1

	metricsPrefix = "stroppy_"

	historySubDir  = "baselines"
	historyDirPerm = 0o755
	historyPerm    = 0o644

	maxReportSuffix = 1000

	statusOK   = "ok"
	statusWarn = "warn"

	medianQ = 0.5
	p90Q    = 0.9
	p99Q    = 0.99

	msPerSecond  = 1000.0
	percentScale = 100.0
)

// errReportNameExhausted reports that a second produced more collision
// suffixes than the naming scheme allows.
var errReportNameExhausted = errors.New("report filename suffixes exhausted")

// HostInfo fingerprints the machine a report was measured on.
type HostInfo struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
	CPUs      int    `json:"cpus"`
	MaxProcs  int    `json:"gomaxprocs"`
}

// LoadStat is one load phase result.
type LoadStat struct {
	Rows       float64 `json:"rows"`
	RowsPerSec float64 `json:"rows_per_second"`
	DurationMs float64 `json:"duration_ms"`
}

// TxStat is one transaction-iteration phase result.
type TxStat struct {
	Iterations float64 `json:"iterations"`
	Failed     float64 `json:"failed_iterations"`
	TxPerSec   float64 `json:"tx_per_second"`
	AvgMs      float64 `json:"avg_ms"`
	P50Ms      float64 `json:"p50_ms"`
	P90Ms      float64 `json:"p90_ms"`
	P99Ms      float64 `json:"p99_ms"`
}

// TierResult collects every phase measured against one driver.
type TierResult struct {
	Name        string   `json:"name"`
	Load        LoadStat `json:"load"`
	TxSingle    TxStat   `json:"tx_single_vu"`
	TxParallel  TxStat   `json:"tx_parallel"`
	ParallelVUs int      `json:"parallel_vus"`
}

// Verdict is one sanity check outcome.
type Verdict struct {
	Check  string `json:"check"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Report is the versioned, machine-readable baseline result written to the
// local history directory.
type Report struct {
	Schema   int          `json:"schema"`
	Stroppy  string       `json:"stroppy_version"`
	PGNoop   string       `json:"pg_noop_version,omitempty"`
	Time     time.Time    `json:"time"`
	Host     HostInfo     `json:"host"`
	Tiers    []TierResult `json:"tiers"`
	Verdicts []Verdict    `json:"verdicts"`
}

// --- metric extraction ---

type histogram struct {
	count   uint64
	sumMs   float64
	bounds  []float64
	buckets []uint64
}

// runMetrics is the slice of one bench.Run's final metrics a report needs.
type runMetrics struct {
	iterations  float64
	failed      float64
	insertRows  float64
	insertDurMs float64
	iter        histogram
}

func extractMetrics(data metricdata.ResourceMetrics) runMetrics {
	var m runMetrics

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			name := strings.TrimPrefix(metric.Name, metricsPrefix)

			switch name {
			case "iterations_total":
				m.iterations = sumFloat(metric.Data)
			case "failed_iterations_total":
				m.failed = sumFloat(metric.Data)
			case "insert_rows_total":
				m.insertRows = sumFloat(metric.Data)
			case "insert_duration":
				m.insertDurMs = histogramOf(metric.Data).sumMs
			case "iteration_duration":
				m.iter = histogramOf(metric.Data)
			}
		}
	}

	return m
}

func sumFloat(data metricdata.Aggregation) float64 {
	sum, ok := data.(metricdata.Sum[float64])
	if !ok {
		return 0
	}

	var total float64
	for _, point := range sum.DataPoints {
		total += point.Value
	}

	return total
}

func histogramOf(data metricdata.Aggregation) histogram {
	hist, ok := data.(metricdata.Histogram[float64])
	if !ok {
		return histogram{}
	}

	var h histogram

	for _, point := range hist.DataPoints {
		h.count += point.Count
		h.sumMs += point.Sum

		if len(h.bounds) == 0 {
			h.bounds = point.Bounds
			h.buckets = make([]uint64, len(point.BucketCounts))
		}

		for i, bucketCount := range point.BucketCounts {
			h.buckets[i] += bucketCount
		}
	}

	return h
}

func (h histogram) avgMs() float64 {
	if h.count == 0 {
		return 0
	}

	return h.sumMs / float64(h.count)
}

// quantile interpolates a quantile from fixed histogram buckets, matching
// bench's summary interpolation.
func (h histogram) quantile(q float64) float64 {
	if h.count == 0 || len(h.buckets) == 0 {
		return 0
	}

	target := uint64(float64(h.count-1)*q) + 1 //nolint:gosec // G115: count-bound

	var cumulative uint64

	for i, bucketCount := range h.buckets {
		cumulative += bucketCount
		if cumulative < target {
			continue
		}

		if i < len(h.bounds) {
			return h.bounds[i]
		}

		if len(h.bounds) > 0 {
			return h.bounds[len(h.bounds)-1]
		}
	}

	return 0
}

// --- verdicts ---

const (
	minScalingEfficiency = 0.6
	maxLoopbackP99Ms     = 1.0
	maxNoiseRatio        = 10.0
)

//nolint:funlen // one function keeps every invariant's threshold beside its check
func evaluate(tiers []TierResult) []Verdict {
	var verdicts []Verdict

	byName := make(map[string]TierResult, len(tiers))
	for idx := range tiers {
		byName[tiers[idx].Name] = tiers[idx]
	}

	for idx := range tiers {
		verdicts = append(verdicts, tierVerdicts(&tiers[idx])...)
	}

	noop, noopOK := byName[tierNoop]
	wire, wireOK := byName[tierWire]

	if wireOK && wire.TxParallel.P99Ms > maxLoopbackP99Ms {
		verdicts = append(verdicts, Verdict{
			Check:  "loopback latency floor",
			Status: statusWarn,
			Detail: fmt.Sprintf("wire p99 %.2fms above %.0fms — VM steal, power saving, or a busy host",
				wire.TxParallel.P99Ms, maxLoopbackP99Ms),
		})
	} else if wireOK {
		verdicts = append(verdicts, Verdict{
			Check:  "loopback latency floor",
			Status: statusOK,
			Detail: fmt.Sprintf("wire p99 %.2fms", wire.TxParallel.P99Ms),
		})
	}

	if noopOK && wireOK && noop.TxParallel.TxPerSec < wire.TxParallel.TxPerSec {
		verdicts = append(verdicts, Verdict{
			Check:  "tier ordering",
			Status: statusWarn,
			Detail: "wire tier outperformed the noop tier — measurement anomaly, rerun",
		})
	}

	return verdicts
}

func tierVerdicts(tier *TierResult) []Verdict {
	var verdicts []Verdict

	failed := tier.TxSingle.Failed + tier.TxParallel.Failed
	if failed > 0 {
		verdicts = append(verdicts, Verdict{
			Check:  tier.Name + " errors",
			Status: statusWarn,
			Detail: fmt.Sprintf("%.0f failed iterations taint the %s tier numbers", failed, tier.Name),
		})
	} else {
		verdicts = append(verdicts, Verdict{
			Check:  tier.Name + " errors",
			Status: statusOK,
			Detail: "no failed iterations",
		})
	}

	if tier.ParallelVUs > 1 && tier.TxSingle.TxPerSec > 0 {
		efficiency := tier.TxParallel.TxPerSec / (tier.TxSingle.TxPerSec * float64(tier.ParallelVUs))
		if efficiency < minScalingEfficiency {
			verdicts = append(verdicts, Verdict{
				Check:  tier.Name + " vu scaling",
				Status: statusWarn,
				Detail: fmt.Sprintf(
					"scaling %.0f%% below %.0f%% — stroppy metric-pipeline contention or machine limits",
					efficiency*percentScale, minScalingEfficiency*percentScale),
			})
		} else {
			verdicts = append(verdicts, Verdict{
				Check:  tier.Name + " vu scaling",
				Status: statusOK,
				Detail: fmt.Sprintf("%.0f%% of linear at %d VUs", efficiency*percentScale, tier.ParallelVUs),
			})
		}
	}

	// Noise quantiles are only meaningful for the wire tier: at noop speeds
	// microsecond quantization drowns the scheduling signal.
	if tier.Name == tierWire && tier.TxParallel.P50Ms > 0 &&
		tier.TxParallel.P99Ms/tier.TxParallel.P50Ms > maxNoiseRatio {
		verdicts = append(verdicts, Verdict{
			Check:  tier.Name + " latency noise",
			Status: statusWarn,
			Detail: fmt.Sprintf("p99/p50 %.1fx above %.0fx — noisy machine or scheduling interference",
				tier.TxParallel.P99Ms/tier.TxParallel.P50Ms, maxNoiseRatio),
		})
	}

	return verdicts
}

// --- history ---

func historyDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(home, ".stroppy", historySubDir), nil
}

func saveReport(report *Report) (string, error) {
	dir, err := historyDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, historyDirPerm); err != nil {
		return "", fmt.Errorf("create history dir: %w", err)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal report: %w", err)
	}

	name := report.Time.UTC().Format("2006-01-02T15-04-05Z")

	//nolint:gosec // G306: user-readable history file
	return writeReportFile(dir, name, append(data, '\n'))
}

// writeReportFile creates <name>.json exclusively; runs saved within the same
// second get a numeric suffix instead of overwriting each other.
func writeReportFile(dir, name string, data []byte) (string, error) {
	for suffix := 1; suffix <= maxReportSuffix; suffix++ {
		fileName := name + ".json"
		if suffix > 1 {
			fileName = fmt.Sprintf("%s-%d.json", name, suffix)
		}

		path := filepath.Join(dir, fileName)

		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, historyPerm)
		if errors.Is(err, fs.ErrExist) {
			continue
		}

		if err != nil {
			return "", fmt.Errorf("create report file: %w", err)
		}

		_, writeErr := file.Write(data)
		closeErr := file.Close()

		if writeErr != nil || closeErr != nil {
			return "", fmt.Errorf("write report: %w", errors.Join(writeErr, closeErr))
		}

		return path, nil
	}

	return "", fmt.Errorf("%w: %s", errReportNameExhausted, name)
}

// loadPrevious returns the newest saved report strictly older than the
// current run. Filenames carry their run time (plus a collision suffix), so
// the current run's own file never matches.
func loadPrevious(current time.Time) (*Report, error) {
	dir, err := historyDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		//nolint:nilnil // no previous report is a valid state
		return nil, nil
	}

	if err != nil {
		return nil, err //nolint:wrapcheck // absent history is surfaced as a plain miss
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(names)))

	for _, name := range names {
		reportTime, ok := reportFileTime(name)
		if !ok || !reportTime.Before(current) {
			continue
		}

		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			continue
		}

		var previous Report
		if jsonErr := json.Unmarshal(data, &previous); jsonErr != nil {
			continue
		}

		return &previous, nil
	}

	//nolint:nilnil // no previous report is a valid state
	return nil, nil
}

// reportFileTime parses the run timestamp encoded in a report filename:
// 2026-09-02T15-04-05Z.json, optionally with a -N collision suffix.
func reportFileTime(name string) (time.Time, bool) {
	name = strings.TrimSuffix(name, ".json")

	if idx := strings.LastIndex(name, "-"); idx > 0 {
		if _, err := strconv.Atoi(name[idx+1:]); err == nil {
			name = name[:idx]
		}
	}

	ts, err := time.Parse("2006-01-02T15-04-05Z", name)
	if err != nil {
		return time.Time{}, false
	}

	return ts, true
}
