package tpcds

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/dsqgen"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/tpcdsgen"
)

var (
	// errCockroachUnsupported refuses TPC-DS on CockroachDB at define time rather than
	// failing eleven queries into a run that has already loaded its data. CRDB does not
	// implement ROLLUP, CUBE or GROUPING SETS (crdb#46280), and 11 of the 103 query
	// bodies use them — exactly the 11 that fail. Its bare ANALYZE is a smaller second
	// blocker, one an asset could fix; the queries cannot be.
	errCockroachUnsupported = errors.New("[tpcds] CockroachDB does not implement ROLLUP/CUBE/GROUPING SETS " +
		"(crdb#46280), which 11 of the 103 queries use, so TPC-DS is not supported there")
	errScaleFactorMustBePositive   = errors.New("SCALE_FACTOR must be positive")
	errUnknownDialect              = errors.New("dsqgen: unknown dialect")
	errIncompleteQuerySet          = errors.New("tpcds: generated query set is incomplete")
	errGeneratedStreamsUnsupported = errors.New("[tpcds] this driver runs the baked query set (power test) only; " +
		"STREAMS>1 and QUERY_STREAM need the in-process generator, which renders a complete " +
		"99-query set for postgres and mysql only")
)

type workload struct {
	schemaSQL *bench.SQL
	querySQL  *bench.SQL
	driver    bench.DriverTypeName
	isPgOrMs  bool
	ydbColumn bool

	scaleFactor   float64
	loadWorkers   int
	useUnlogged   bool
	ydbStoreMode  string
	schemaFile    string
	sqlFile       string
	validateForce bool

	throughput bool
	streams    int
	seed       int64
	genStream  int // QUERY_STREAM value (<0 = unset → baked)

	validation validationReport
}

type namedQuery struct {
	name string
	sql  string
}

var Test = bench.Test{Name: "tpcds", Define: define, Source: publication("Test")}

func init() { bench.Register(Test) }

// tpcdsSupported reports whether TPC-DS can run on dt at all.
//
// CockroachDB cannot: it does not implement ROLLUP, CUBE or GROUPING SETS (its issue
// #46280), and eleven of the 103 query bodies use one of them. Nothing an asset can do
// changes that, so the workload refuses at define time instead of failing mid-run.
func tpcdsSupported(dt bench.DriverTypeName) bool { return dt != bench.DriverCRDB }

//nolint:gocognit,cyclop,funlen // query choices precede schema-changing actions.
func define(d *bench.Def) error {
	settings := bench.RunParameters(&d.Param, bench.RunDefaults{})
	w := &workload{}
	d.Report.Contribute("tpcds.validation", 1, w.validationContribution)
	w.scaleFactor, _ = d.Param.Float64("scale-factor", 1, "TPC-DS scale factor.")
	w.loadWorkers, _ = d.Param.Int("load-workers", 0, "Workers used to load each table.")
	w.useUnlogged, _ = d.Param.Bool("pg-unlogged", false, "Use unlogged tables while loading.")
	w.ydbStoreMode, _ = d.Param.String("ydb-store-mode", "column", "YDB table store mode.")
	w.streams, _ = d.Param.Int("streams", 1, "Number of query streams.")
	w.seed, _ = d.Param.Int64("query-seed", 19620718, "Query generator seed.")
	stream, streamInfo := d.Param.Int("query-stream", 0, "Query stream to generate.")

	w.genStream = -1
	if streamInfo.Explicit() {
		w.genStream = stream
	}

	w.schemaFile, _ = d.Param.String("schema-file", "", "Schema SQL file override.")
	w.sqlFile, _ = d.Param.String("sql-file", "", "Query SQL file override.")

	w.validateForce, _ = d.Param.Bool("validate-force", false, "Validate outside scale factor 1.")
	if w.scaleFactor <= 0 {
		return fmt.Errorf("%w, got %v", errScaleFactorMustBePositive, w.scaleFactor)
	}

	db := d.Drivers.Declare("default", bench.DriverConfig{})
	w.driver = db.Kind()

	if !tpcdsSupported(w.driver) {
		return errCockroachUnsupported
	}

	w.isPgOrMs = w.driver == bench.DriverPostgres || w.driver == bench.DriverMySQL
	w.ydbColumn = w.driver == bench.DriverYDB && w.ydbStoreMode == "column"
	w.useUnlogged = w.useUnlogged && w.driver == bench.DriverPostgres

	w.throughput = w.streams > 1
	// Only drivers whose generator renders the full 99-query set can build a stream.
	// The mysql generator skips q51 and q97 and generateStream rejects a skipped set,
	// and the postgres streams carry ROLLUP and window functions that pico.sql
	// documents as unsupported. Those engines run the baked set only, and saying so at
	// definition is the difference between a refusal and a load followed by a failure —
	// but only where a stream is wanted at all: a run that excludes the workload step
	// never generates one, and loading data on those engines stays legitimate.
	streamsWanted := (w.throughput || w.genStream >= 0) && d.Execution.Enabled("workload")

	if streamsWanted && !generatedStreamsSupported(w.driver) {
		return errGeneratedStreamsUnsupported
	}

	if streamsWanted {
		if _, err := w.streamQueries(max(w.genStream, 0)); err != nil {
			return err
		}
	}

	schemaFile, queryFile := dialectFiles(w.driver, w.schemaFile, w.sqlFile)

	var err error
	if w.schemaFile != "" {
		w.schemaSQL, err = d.Queries.Override(schemaFile)
	} else {
		w.schemaSQL, err = d.Queries.Load(files, schemaFile)
	}

	if err != nil {
		return err
	}

	if w.sqlFile != "" {
		w.querySQL, err = d.Queries.Override(queryFile)
	} else {
		w.querySQL, err = d.Queries.Load(files, queryFile)
	}

	if err != nil {
		return err
	}

	d.Execution.Step("drop_schema", w.drop)
	d.Execution.Step("create_schema", w.create)

	if w.useUnlogged {
		d.Execution.Step("set_unlogged", w.unlogged)
	}

	d.Execution.Step("load_data", w.load)
	section := schemaSection{w, "create_indexes"}
	d.Execution.Step(section.name, section.Run)

	if w.useUnlogged {
		d.Execution.Step("set_logged", w.logged)
	}

	d.Execution.Step("analyze", w.analyzeAction)

	if !w.throughput && w.genStream < 0 && w.isPgOrMs {
		d.Execution.Step("validate_answers", w.validate)
	}

	d.Execution.Step("workload", w.work, settings.Policy())

	return d.Execution.Err()
}

func (w *workload) drop(ctx context.Context, b *bench.Bench) error { return w.dropSchema(ctx, b)() }

func (w *workload) create(ctx context.Context, b *bench.Bench) error { return w.createSchema(ctx, b)() }

func (w *workload) unlogged(ctx context.Context, b *bench.Bench) error {
	return w.setUnlogged(ctx, b, "UNLOGGED")()
}

func (w *workload) logged(ctx context.Context, b *bench.Bench) error {
	return w.setUnlogged(ctx, b, "LOGGED")()
}

func (w *workload) analyzeAction(ctx context.Context, b *bench.Bench) error {
	return w.analyze(ctx, b)()
}

type schemaSection struct {
	workload *workload
	name     string
}

func (s schemaSection) Run(ctx context.Context, b *bench.Bench) error {
	return s.workload.runSection(ctx, b, s.workload.schemaSQL, s.name)
}

func (w *workload) load(ctx context.Context, b *bench.Bench) error {
	for _, table := range tpcdsTables {
		source, err := tpcdsgen.NewBatchSource(table, w.scaleFactor)
		if err != nil {
			return err
		}

		if _, err = b.Insert(ctx, table, source, bench.LoadWorkers(max(w.loadWorkers, 1))); err != nil {
			return fmt.Errorf("load %s: %w", table, err)
		}
	}

	return nil
}

func (w *workload) validate(ctx context.Context, b *bench.Bench) error {
	w.validation = validateAnswers(
		ctx,
		b,
		w.schemaSQL,
		w.querySQL,
		w.querySQL.Names(""),
		w.scaleFactor,
		w.driver,
		w.validateForce,
	)

	return nil
}

// generatedStreamsSupported reports whether dsqgen can render a complete query stream
// for this driver. TiDB is excluded despite speaking mysql: the mysql generator skips
// q51 and q97, and generateStream refuses a set with skips in it. Picodata is excluded
// because the postgres streams carry ROLLUP and window functions its asset documents
// as unsupported.
func generatedStreamsSupported(dt bench.DriverTypeName) bool {
	switch dt {
	case bench.DriverPostgres, bench.DriverMySQL, bench.DriverYugabyte:
		return true
	default:
		return false
	}
}

func (w *workload) work(ctx context.Context, b *bench.Bench) error {
	queries, err := w.resolveQueries(b)
	if err != nil {
		return err
	}

	for _, q := range queries {
		start := time.Now()

		if err := b.Exec(ctx, q.sql, nil); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			b.RecordError(q.name, err)

			continue
		}

		b.Log.Info(
			"query completed",
			"query",
			q.name,
			"elapsed",
			time.Since(start),
		)
	}

	return nil
}

// resolveQueries returns this VU's query list. Throughput: VU N runs generated stream N.
// Explicit QUERY_STREAM: that stream. Otherwise the baked canonical set.
func (w *workload) resolveQueries(b *bench.Bench) ([]namedQuery, error) {
	if w.genStream < 0 && !w.throughput {
		names := w.querySQL.Names("")

		out := make([]namedQuery, 0, len(names))
		for _, name := range names {
			if body, ok := w.querySQL.Query("", name); ok {
				out = append(out, namedQuery{name, body})
			}
		}

		return out, nil
	}

	var streamIdx int
	if w.throughput {
		streamIdx = b.Worker()
	} else {
		streamIdx = w.genStream
	}

	return w.streamQueries(streamIdx)
}

// streamQueries renders one query stream for this workload's dialect.
//
// The definition-time preflight and the workload step both need it, and each of them
// remembering the mapping is how #218 happened: the dialect was translated at one call
// site and the kind itself passed at the other, so a run that reached only the second
// failed on a dialect dsqgen has never heard of.
func (w *workload) streamQueries(stream int) ([]namedQuery, error) {
	return generateStream(dsqgenDialect(w.driver), w.scaleFactor, w.seed, stream)
}

// dsqgenDialect maps a driver kind to the dialect name dsqgen knows. The borrowed
// engines speak one of the two protocols, and passing the kind itself — "tidb",
// "yugabyte" — fails at define time because dsqgen has no such dialect. #218.
func dsqgenDialect(dt bench.DriverTypeName) string {
	switch dt {
	case bench.DriverMySQL, bench.DriverTiDB:
		return "mysql"
	default:
		return "postgres"
	}
}

// generateStream renders one TPC-DS query stream in process.
func generateStream(dialect string, scale float64, seed int64, stream int) ([]namedQuery, error) {
	d, ok := dsqgen.DialectByName(dialect)
	if !ok {
		return nil, fmt.Errorf("%w %q", errUnknownDialect, dialect)
	}

	res, err := dsqgen.Generate(d, scale, seed, stream)
	if err != nil {
		return nil, err
	}

	if len(res.Skipped) > 0 {
		return nil, fmt.Errorf(
			"%w: %s",
			errIncompleteQuerySet,
			strings.Join(res.Skipped, "; "),
		)
	}

	if len(res.Queries) != 99 {
		return nil, fmt.Errorf(
			"%w: expected 99 queries, got %d",
			errIncompleteQuerySet,
			len(res.Queries),
		)
	}

	suffix := []string{"_a", "_b", "_c"}

	out := make([]namedQuery, 0, len(res.Queries))
	for _, q := range res.Queries {
		stmts := strings.Split(q.SQL, ";")

		var n int

		for _, s := range stmts {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}

			name := q.Name
			if len(stmts) > 1 && n < len(suffix) {
				name += suffix[n]
			}

			out = append(out, namedQuery{name, s})
			n++
		}
	}

	return out, nil
}

// --- setup helpers ---

func (w *workload) runSection(ctx context.Context, b *bench.Bench, sql *bench.SQL, section string) error {
	for _, q := range sql.Section(section) {
		if err := b.Exec(ctx, q, nil); err != nil {
			return fmt.Errorf("%s: %w", section, err)
		}
	}

	return nil
}

func (w *workload) dropSchema(ctx context.Context, b *bench.Bench) func() error {
	return func() error {
		// ydb/picodata have no CASCADE; drop from the schema file's drop_schema section.
		if w.driver == bench.DriverYDB || w.driver == bench.DriverPicodata {
			return w.runSection(ctx, b, w.schemaSQL, "drop_schema")
		}
		// pg/mysql: reverse load order, CASCADE (mysql accepts/ignores the keyword).
		for i := len(tpcdsTables) - 1; i >= 0; i-- {
			if err := b.Exec(
				ctx,
				fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", tpcdsTables[i]),
				nil,
			); err != nil {
				return fmt.Errorf("drop_schema: %w", err)
			}
		}

		return nil
	}
}

func (w *workload) createSchema(ctx context.Context, b *bench.Bench) func() error {
	return func() error {
		section := "create_schema"
		if w.ydbColumn {
			section = "create_schema_column"
		}

		return w.runSection(ctx, b, w.schemaSQL, section)
	}
}

func (w *workload) setUnlogged(ctx context.Context, b *bench.Bench, mode string) func() error {
	return func() error {
		for _, table := range tpcdsTables {
			if err := b.Exec(ctx, fmt.Sprintf("ALTER TABLE %s SET %s", table, mode), nil); err != nil {
				return fmt.Errorf("set_%s %s: %w", strings.ToLower(mode), table, err)
			}
		}

		return nil
	}
}

func (w *workload) analyze(ctx context.Context, b *bench.Bench) func() error {
	return func() error {
		switch w.driver {
		case bench.DriverPostgres, bench.DriverYugabyte:
			return b.Exec(ctx, "ANALYZE", nil)
		case bench.DriverCRDB:
			// CockroachDB rejects a bare ANALYZE and takes one table at a time. TPC-DS is
			// otherwise unsupported there — its rollup queries are unimplemented (CRDB
			// issue #46280) — but this keeps the step honest if someone tries.
			for _, table := range tpcdsTables {
				if err := b.Exec(ctx, "ANALYZE "+table, nil); err != nil {
					return fmt.Errorf("analyze %s: %w", table, err)
				}
			}
		case bench.DriverMySQL, bench.DriverTiDB:
			for _, table := range tpcdsTables {
				if err := b.Exec(ctx, "ANALYZE TABLE "+table, nil); err != nil {
					return fmt.Errorf("analyze %s: %w", table, err)
				}
			}
		case bench.DriverPicodata, bench.DriverYDB, bench.DriverNoop, bench.DriverCSV, bench.DriverRecording:
			// no ANALYZE; planner runs on index stats from create_indexes.
		}

		return nil
	}
}
