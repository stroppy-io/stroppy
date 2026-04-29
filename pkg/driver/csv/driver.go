// Package csv implements an ephemeral Stroppy driver that writes
// generator output to CSV files on the local filesystem instead of a
// database. It exists to (a) benchmark pure generation throughput
// without database I/O, (b) produce reference output for cross-tool
// comparisons, and (c) feed downstream systems that bulk-load from
// delimited files (ClickHouse, DuckDB, PostgreSQL COPY, etc.).
//
// Configuration is entirely URL-driven. The path component of the URL
// selects the output directory (defaults to the current working
// directory when absent) and the query string carries the small set of
// supported knobs: ?merge=true|false, ?separator=comma|tab,
// ?header=true|false.
//
// The driver implements only the relational InsertSpec NATIVE path.
// Every other InsertMethod is rejected with ErrUnsupportedInsertMethod;
// runtime query execution is rejected with ErrCsvDriverNoQuery. DDL
// emitted by the drop_schema and create_schema workload steps is
// accepted and processed out-of-band: DROP clauses delete the
// workload's output directory for idempotent reruns, CREATE is a noop.
package csv

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/common/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
)

// csvBufferSize bounds the bufio.Writer wrapping each shard's
// csv.Writer. 64 KiB is the stroppy-wide I/O buffer default and
// large enough to amortize the per-row flush cost without holding
// unbounded memory per worker.
const csvBufferSize = 64 * 1024

// Filesystem permissions for directories the driver creates (owner
// rwx, group/other rx) and the MANIFEST.json it emits (owner rw,
// group/other r). Broken out as constants to avoid magic numbers.
const (
	dirMode  = 0o755
	fileMode = 0o644
)

// Accepted boolean-option strings. parseConfig applies them to any
// ?merge / ?header query param. Everything else returns
// ErrInvalidOption.
var (
	boolTrue  = map[string]struct{}{"true": {}, "1": {}, "yes": {}}
	boolFalse = map[string]struct{}{"false": {}, "0": {}, "no": {}}
)

// ErrInvalidOption is the static parent error for any invalid URL
// query value. The concrete per-option message wraps it.
var ErrInvalidOption = errors.New("csv: invalid URL option")

// config holds the parsed URL options for one CSV driver instance.
type config struct {
	// dir is the absolute output directory root. Every workload's CSVs
	// land under dir/<workload>/.
	dir string
	// separator is one of separatorComma or separatorTab.
	separator rune
	// header is true when the driver must emit a header row for each
	// table (default). With merge=false the header is written to a
	// sidecar <table>.header.csv so worker shards stay header-free.
	header bool
	// merge requests post-load shard concatenation into a single
	// <table>.csv per table. merge=false leaves worker shards in place
	// at <outdir>/<workload>/<table>.w%03d.csv for downstream tools
	// that accept glob inputs.
	merge bool
	// workload pins the workload sub-directory. Empty means "fall
	// back to STROPPY_CSV_WORKLOAD env var, then 'default'."
	workload string
}

func init() {
	driver.RegisterDriver(
		stroppy.DriverConfig_DRIVER_TYPE_CSV,
		func(ctx context.Context, opts driver.Options) (driver.Driver, error) {
			return NewDriver(ctx, opts)
		},
	)
}

// Driver emits generator rows to CSV files. One Driver instance is
// scoped to one k6 run; tables accumulate under a single output
// directory and are either merged or left as worker shards at
// Teardown.
type Driver struct {
	logger *zap.Logger
	cfg    config

	// workloadDir is computed at first InsertSpec or DDL observation
	// and kept stable for the life of the driver. Filesystem layout:
	// <cfg.dir>/<workload>/.shards/<table>.w%03d.csv when merge=true,
	// or <cfg.dir>/<workload>/<table>.w%03d.csv when merge=false.
	workloadDir  string
	workloadName string

	// tables records the tables that had rows written during this run
	// so Teardown can merge or finalize them. Guarded by mu.
	mu     sync.Mutex
	tables map[string]*tableState
}

// tableState is the per-table bookkeeping kept during a run: how many
// shards were opened and the cumulative row count. column order is
// taken from the runtime at first emission and used once per table by
// the merge pass to build the header.
type tableState struct {
	columns  []string
	shards   int
	rowCount int64
}

var _ driver.Driver = (*Driver)(nil)

// NewDriver parses opts.Config.Url and returns a ready-to-use Driver.
// The output directory is created lazily (on first write) so Setup
// succeeds even when dir is a prefix that does not yet exist.
func NewDriver(_ context.Context, opts driver.Options) (*Driver, error) {
	lg := opts.Logger
	if lg == nil {
		lg = logger.NewFromEnv().Named("csv")
	}

	cfg, err := parseConfig(opts.Config.GetUrl())
	if err != nil {
		return nil, fmt.Errorf("csv: parse url: %w", err)
	}

	lg.Debug("csv driver configured",
		zap.String("dir", cfg.dir),
		zap.Bool("merge", cfg.merge),
		zap.Bool("header", cfg.header),
		zap.String("separator", string(cfg.separator)),
	)

	return &Driver{
		logger: lg,
		cfg:    cfg,
		tables: make(map[string]*tableState),
	}, nil
}

// defaultConfig returns the config that an empty URL produces.
func defaultConfig() config {
	return config{
		separator: ',',
		header:    true,
		merge:     true,
	}
}

// parseConfig turns a raw URL string into a config. The path component
// (everything before '?') is the output directory; the query component
// supplies optional knobs. An empty URL resolves to the current working
// directory with all-defaults options.
func parseConfig(raw string) (config, error) {
	cfg := defaultConfig()

	if raw == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return cfg, fmt.Errorf("resolve cwd: %w", err)
		}

		cfg.dir = cwd

		return cfg, nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return cfg, fmt.Errorf("url.Parse(%q): %w", raw, err)
	}

	dir, err := resolveDir(parsed)
	if err != nil {
		return cfg, err
	}

	cfg.dir = dir

	if err := applyQuery(&cfg, parsed.Query()); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// resolveDir returns the absolute output directory derived from the
// URL's path / opaque component, falling back to the current working
// directory when neither is set.
func resolveDir(parsed *url.URL) (string, error) {
	path := parsed.Path
	if path == "" {
		path = parsed.Opaque
	}

	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve cwd: %w", err)
		}

		path = cwd
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve abs path %q: %w", path, err)
	}

	return absPath, nil
}

// applyQuery folds every supported query parameter into cfg. An
// invalid value on any parameter returns ErrInvalidOption wrapped
// with the offending field.
func applyQuery(cfg *config, query url.Values) error {
	if v := query.Get("merge"); v != "" {
		b, err := parseBool("merge", v)
		if err != nil {
			return err
		}

		cfg.merge = b
	}

	if v := query.Get("header"); v != "" {
		b, err := parseBool("header", v)
		if err != nil {
			return err
		}

		cfg.header = b
	}

	if v := query.Get("separator"); v != "" {
		sep, err := parseSeparator(v)
		if err != nil {
			return err
		}

		cfg.separator = sep
	}

	if v := query.Get("workload"); v != "" {
		cfg.workload = v
	}

	return nil
}

// parseBool accepts the well-known truthy/falsy strings. An unknown
// value returns ErrInvalidOption wrapped with the field name.
func parseBool(field, raw string) (bool, error) {
	lc := strings.ToLower(raw)
	if _, ok := boolTrue[lc]; ok {
		return true, nil
	}

	if _, ok := boolFalse[lc]; ok {
		return false, nil
	}

	return false, fmt.Errorf("%w: %s=%q (want true|false)", ErrInvalidOption, field, raw)
}

// parseSeparator maps the user-facing separator names to their rune
// values. Only comma and tab are supported.
func parseSeparator(raw string) (rune, error) {
	switch strings.ToLower(raw) {
	case "comma", ",":
		return ',', nil
	case "tab", "\\t":
		return '\t', nil
	default:
		return 0, fmt.Errorf("%w: separator=%q (want comma|tab)", ErrInvalidOption, raw)
	}
}

// resolveWorkload pins the workload sub-directory on first use. The
// workload name comes from the URL's ?workload= query parameter when
// present, else from the STROPPY_CSV_WORKLOAD env var, else
// "default". We cannot infer from the spec alone because InsertSpecs
// know their table name, not the workload grouping.
func (d *Driver) resolveWorkload() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.workloadDir != "" {
		return d.workloadDir
	}

	name := d.cfg.workload
	if name == "" {
		name = os.Getenv("STROPPY_CSV_WORKLOAD")
	}

	if name == "" {
		name = "default"
	}

	d.workloadName = name
	d.workloadDir = filepath.Join(d.cfg.dir, name)

	return d.workloadDir
}

// Teardown finalizes the run: merges shards when configured, or emits
// a sidecar header when merge=false. Safe to call multiple times; all
// operations are idempotent.
func (d *Driver) Teardown(_ context.Context) error {
	d.mu.Lock()

	if d.workloadDir == "" {
		d.mu.Unlock()

		return nil
	}

	snapshot := make(map[string]*tableState, len(d.tables))

	for name, ts := range d.tables {
		cp := *ts
		snapshot[name] = &cp
	}

	workloadDir := d.workloadDir
	workloadName := d.workloadName

	d.mu.Unlock()

	if d.cfg.merge {
		if err := d.mergeAll(workloadDir, snapshot); err != nil {
			return err
		}
	} else {
		if err := d.emitHeaderSidecars(workloadDir, snapshot); err != nil {
			return err
		}
	}

	if err := writeManifest(workloadDir, workloadName, d.cfg, snapshot); err != nil {
		return fmt.Errorf("csv: write manifest: %w", err)
	}

	d.logger.Debug("csv teardown complete",
		zap.String("dir", workloadDir),
		zap.Int("tables", len(snapshot)),
	)

	return nil
}
