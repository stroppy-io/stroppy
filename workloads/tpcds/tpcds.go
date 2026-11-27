package tpcds

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/mpp"
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
	errMariaDBDriver               = errors.New("sql-dialect mariadb requires the mysql driver")
	errScaleFactorMustBePositive   = errors.New("SCALE_FACTOR must be positive")
	errUnknownDialect              = errors.New("dsqgen: unknown dialect")
	errIncompleteQuerySet          = errors.New("tpcds: generated query set is incomplete")
	errGeneratedStreamsUnsupported = errors.New("[tpcds] this driver runs the baked query set (power test) only; " +
		"STREAMS>1 and QUERY_STREAM need the in-process generator, which renders a complete " +
		"99-query set for postgres, mysql, yugabyte and tidb only")
)

type workload struct {
	schemaSQL  *bench.SQL
	querySQL   *bench.SQL
	driver     bench.DriverTypeName
	ydbColumn  bool
	sqlDialect string

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

	validation     validationReport
	metrics        map[string]*queryMetrics
	benchmark      *benchmarkState
	maintenanceSQL *bench.SQL
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
	benchmark, benchmarkInfo := d.Param.Bool("benchmark", false,
		"Run load, power, two throughput tests and data maintenance; report QphDS@SF.")

	mode, modeInfo := d.Param.String("mode", "legacy", "Execution graph mode; legacy preserves benchmark selection.",
		bench.OneOf("legacy", "queries", "benchmark"))
	if modeInfo.Explicit() && mode != "legacy" {
		if benchmarkInfo.Explicit() && benchmark != (mode == "benchmark") {
			return errBenchmarkOptions
		}

		benchmark = mode == "benchmark"
	}

	refreshSet, refreshInfo := d.Param.Int64("refresh-set", 1,
		"First maintenance set; required when continuing existing data.", bench.Min(int64(1)))
	streamDefault := benchmarkStreamDefault(benchmark)

	d.Report.Contribute("tpcds.validation", 1, w.validationContribution)
	w.scaleFactor, _ = d.Param.Float64("scale-factor", 1, "TPC-DS scale factor.")
	w.loadWorkers, _ = d.Param.Int("load-workers", 0, "Workers used to load each table.")
	w.useUnlogged, _ = d.Param.Bool("pg-unlogged", false, "Use unlogged tables while loading.")
	w.ydbStoreMode, _ = d.Param.String("ydb-store-mode", "column", "YDB table store mode.")
	w.streams, _ = d.Param.Int("streams", streamDefault, "Number of query streams.", bench.Min(1))
	w.seed, _ = d.Param.Int64("query-seed", 19620718, "Query generator seed.")
	stream, streamInfo := d.Param.Int("query-stream", 0, "Query stream to generate.")

	w.genStream = -1
	if streamInfo.Explicit() {
		w.genStream = stream
	}

	w.schemaFile, _ = d.Param.String("schema-file", "", "Schema SQL file override.")
	w.sqlFile, _ = d.Param.String("sql-file", "", "Query SQL file override.")
	dialect, _ := d.Param.String("sql-dialect", "",
		"Built-in SQL dialect: pg, gp (Greenplum family), mariadb (mysql driver); empty selects by driver.",
		bench.OneOf("", "pg", mpp.Dialect, "mariadb"))

	w.validateForce, _ = d.Param.Bool("validate-force", false, "Validate outside scale factor 1.")
	if w.scaleFactor <= 0 {
		return fmt.Errorf("%w, got %v", errScaleFactorMustBePositive, w.scaleFactor)
	}

	db := d.Drivers.Declare("default", bench.DriverConfig{})
	w.driver = db.Kind()

	if !tpcdsSupported(w.driver) {
		return errCockroachUnsupported
	}

	w.ydbColumn = w.driver == bench.DriverYDB && w.ydbStoreMode == "column"

	w.sqlDialect = dialect
	if dialect == "mariadb" {
		if w.driver != bench.DriverMySQL && w.driver != bench.DriverNoop && w.driver != bench.DriverRecording {
			return errMariaDBDriver
		}

		dialect = ""
	}

	dialectChoice, err := mpp.Select(mpp.Request{
		Dialect: dialect, Driver: w.driver, OwnSchema: w.schemaFile != "", Unlogged: w.useUnlogged,
	})
	if err != nil {
		return err
	}

	w.useUnlogged = w.useUnlogged && w.driver == bench.DriverPostgres

	w.throughput = w.streams > 1
	// Check only runs that execute generated queries: load-only runs need no generator.
	streamsWanted := (w.throughput || w.genStream >= 0) && d.Execution.Enabled("workload")

	if streamsWanted && !generatedStreamsSupported(w.driver) {
		return errGeneratedStreamsUnsupported
	}

	if streamsWanted && !benchmark {
		if _, err := w.streamQueries(max(w.genStream, 0)); err != nil {
			return err
		}
	}

	schemaFile, queryFile := w.sqlFiles(dialectChoice.GP())

	if err := w.loadSQL(d, schemaFile, queryFile); err != nil {
		return err
	}

	w.metrics = declareQueryMetrics(&d.Metrics, w.querySQL.Names(""))

	return w.executeGraph(d, dialectChoice, settings, benchmark, refreshSet, refreshInfo.Explicit())
}

func (w *workload) executeGraph(
	d *bench.Def, dialectChoice *mpp.Selection, settings bench.RunSettings,
	benchmark bool, refreshSet int64, refreshExplicit bool,
) error {
	g := d.Execution.Graph("tpcds")

	var previous, members []bench.NodeRef

	step := func(name string, action bench.Action, options ...bench.StepOption) bench.NodeRef {
		node := g.Node(name, dialectChoice.Step(action), options...).After(previous...)
		previous = []bench.NodeRef{node}
		members = append(members, node)

		return node
	}
	benchmarkWork := w

	if !benchmark {
		candidate := *w
		benchmarkWork = &candidate
	}

	benchmarkErr := benchmarkWork.configureBenchmark(d, settings, refreshSet, refreshExplicit, benchmark)
	if benchmark && benchmarkErr != nil {
		return benchmarkErr
	}
	defer func() {
		if benchmarkWork.benchmark != nil && benchmarkWork.benchmark.directory != "" {
			_ = os.RemoveAll(benchmarkWork.benchmark.directory)
		}
	}()

	var maintenance bench.NodeRef
	if benchmarkErr == nil {
		maintenance = g.Node("generate_maintenance", dialectChoice.Step(benchmarkWork.generateMaintenance))
		if benchmark {
			previous = []bench.NodeRef{maintenance}
		}
	}

	step("drop_schema", w.drop)
	step("create_schema", w.create)

	w.executeLoad(step)

	setup := slices.Clone(members)
	query := g.Node("workload", dialectChoice.Step(w.work), settings.Policy()).After(previous...)
	g.Mode("queries", "Query workload with optional setup and qualification", append(slices.Clone(setup), query)...)

	benchmarkMembers := append([]bench.NodeRef{maintenance}, setup...)

	if benchmarkErr == nil {
		benchmarkStep := func(name string, action bench.Action, options ...bench.StepOption) bench.NodeRef {
			node := g.Node(name, dialectChoice.Step(action), options...).After(previous...)
			previous = []bench.NodeRef{node}
			benchmarkMembers = append(benchmarkMembers, node)

			return node
		}
		benchmarkWork.declareBenchmark(g, benchmarkStep, dialectChoice.Step, &previous, &benchmarkMembers)
		g.Mode("benchmark", "Power, two throughput tests and data maintenance", benchmarkMembers...)
	} else {
		g.Mode("benchmark", "Power, two throughput tests and data maintenance").Require(benchmarkErr)
	}

	selected := "queries"
	if benchmark {
		selected = "benchmark"
	}

	return g.Run(selected)
}

func (w *workload) sqlFiles(gp bool) (schemaFile, queryFile string) {
	schemaFile, queryFile = dialectFiles(w.driver, gp, w.schemaFile, w.sqlFile)
	if w.sqlDialect == "mariadb" {
		if w.schemaFile == "" {
			schemaFile = "schema.mysql.sql"
		}

		if w.sqlFile == "" {
			queryFile = "mariadb.sql"
		}
	}

	return schemaFile, queryFile
}

func (w *workload) loadSQL(d *bench.Def, schemaFile, queryFile string) error {
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

	return err
}

func (w *workload) executeLoad(step func(string, bench.Action, ...bench.StepOption) bench.NodeRef) {
	if w.useUnlogged {
		step("set_unlogged", w.unlogged)
	}

	step("load_data", w.load)
	section := schemaSection{w, "create_indexes"}
	step(section.name, section.Run)

	if w.useUnlogged {
		step("set_logged", w.logged)
	}

	step("analyze", w.analyzeAction)

	if !w.throughput && w.genStream < 0 && answerValidationSupported(w.driver) {
		step("validate_answers", w.validate)
	}
}

func (w *workload) drop(ctx context.Context, b *bench.Bench) error {
	if w.benchmark != nil {
		if err := b.Exec(ctx, "DROP TABLE IF EXISTS stroppy_tpcds_refresh", nil); err != nil {
			return err
		}
	}

	return w.dropSchema(ctx, b)()
}

func (w *workload) create(ctx context.Context, b *bench.Bench) error {
	if err := w.createSchema(ctx, b)(); err != nil {
		return err
	}

	if w.benchmark != nil {
		return b.Exec(ctx, "DROP TABLE IF EXISTS stroppy_tpcds_refresh", nil)
	}

	return nil
}

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
	total := float64(len(tpcdsTables))
	for index, table := range tpcdsTables {
		b.Progress(ctx, bench.ProgressUpdate{Activity: table, Unit: "tables", Current: float64(index), Total: &total})

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

// Picodata and YDB need their baked rewrites for constructs in generated streams.
func generatedStreamsSupported(dt bench.DriverTypeName) bool {
	switch dt {
	case bench.DriverPostgres, bench.DriverMySQL, bench.DriverYugabyte, bench.DriverTiDB:
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

	total := float64(len(queries))
	for index, q := range queries {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		b.Progress(ctx, bench.ProgressUpdate{
			Activity: queryProgressName(q.name), Unit: "queries", Current: float64(index), Total: &total,
		})

		start := time.Now()
		err := b.Exec(ctx, q.sql, nil)
		elapsed := time.Since(start)

		b.Progress(ctx, bench.ProgressUpdate{
			Activity: queryProgressName(q.name), Unit: "queries", Current: float64(index + 1), Total: &total,
		})

		w.metrics[q.name].record(ctx, elapsed, err != nil)

		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			b.RecordError(q.name, err)

			continue
		}

		b.Log.Info("query completed", "query", q.name, "elapsed", elapsed)
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
	dialect := dsqgenDialect(w.driver)
	if w.sqlDialect == "mariadb" {
		dialect = "mariadb"
	}

	return generateStream(dialect, w.scaleFactor, w.seed, stream)
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
		stmts := strings.Split(strings.TrimRight(strings.TrimSpace(q.SQL), ";"), ";")

		var n int

		for _, s := range stmts {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}

			name := canonicalQueryName(q.Name)
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
	names := sql.Names(section)
	for i, q := range sql.Section(section) {
		started := time.Now()
		if err := b.Exec(ctx, q, nil); err != nil {
			return fmt.Errorf("%s: statement %d (%q), driver %s, elapsed %s: %w",
				section, i+1, names[i], b.DriverTypeName(), time.Since(started), err)
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
