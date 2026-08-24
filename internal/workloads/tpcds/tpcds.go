package tpcds

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/pkg/bench"
	"github.com/stroppy-io/stroppy/third_party/gotpcds/dsqgen"
)

var (
	errScaleFactorMustBePositive = errors.New("SCALE_FACTOR must be positive")
	errUnknownDialect            = errors.New("dsqgen: unknown dialect")
	errYdbBakedOnly              = errors.New("[tpcds] ydb supports the baked query set (power test) only; " +
		"STREAMS>1 and QUERY_STREAM need the in-process generator, which does not target YQL yet")
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
}

type namedQuery struct {
	name string
	sql  string
}

func init() { bench.Register(func() bench.Workload { return &workload{} }) }

func (*workload) Name() string { return "tpcds" }

func (w *workload) Define(d *bench.Def) error {
	w.scaleFactor = d.Param.Float64("scale-factor", 1, "TPC-DS scale factor.").Value()
	w.loadWorkers = d.Param.Int("load-workers", 0, "Workers used to load each table.").Value()
	w.useUnlogged = d.Param.Bool("pg-unlogged", false, "Use unlogged PostgreSQL tables while loading.").Value()
	w.ydbStoreMode = d.Param.String("ydb-store-mode", "column", "YDB table store mode.").Value()
	w.streams = d.Param.Int("streams", 1, "Number of query streams.").Value()
	w.seed = int64(d.Param.Int("query-seed", 19620718, "Query generator seed.").Value())

	queryStream := d.Param.Int("query-stream", 0, "Query stream to generate.")
	if queryStream.Explicit() {
		w.genStream = queryStream.Value()
	} else {
		w.genStream = -1
	}

	w.schemaFile = d.Param.String("schema-file", "", "Schema SQL file override.").Value()
	w.sqlFile = d.Param.String("sql-file", "", "Query SQL file override.").Value()
	validateForce := d.Param.Bool("validate-force", false, "Validate answers outside scale factor 1.")

	w.validateForce = validateForce.Value()

	legacyForce := validateForce.Source() == bench.ParamSourceProcessEnv ||
		validateForce.Source() == bench.ParamSourceLegacyEnv ||
		validateForce.Source() == bench.ParamSourceLegacyConfigEnv
	if validateForce.Explicit() && legacyForce {
		w.validateForce = true
	}

	if w.scaleFactor <= 0 {
		return fmt.Errorf("%w, got %v", errScaleFactorMustBePositive, w.scaleFactor)
	}

	return nil
}

func (w *workload) Setup(ctx context.Context, b *bench.Bench) error {
	w.driver = b.DriverTypeName()
	w.isPgOrMs = w.driver == bench.DriverPostgres || w.driver == bench.DriverMySQL
	w.ydbColumn = w.driver == bench.DriverYDB && w.ydbStoreMode == "column"
	w.useUnlogged = w.useUnlogged && w.driver == bench.DriverPostgres

	// Query source. Throughput (STREAMS>1) or an explicit QUERY_STREAM selects the
	// in-process generator; otherwise the baked canonical set. ydb runs the baked set
	// only — the generator targets ANSI/pg/MySQL, not YQL.
	w.throughput = w.streams > 1

	if w.driver == bench.DriverYDB && (w.throughput || w.genStream >= 0) {
		return errYdbBakedOnly
	}

	schemaFile, queryFile := dialectFiles(w.driver, w.schemaFile, w.sqlFile)
	w.schemaSQL = mustLoad(preset, schemaFile)
	w.querySQL = mustLoad(preset, queryFile)

	if err := w.runSteps(ctx, b); err != nil {
		return err
	}

	return nil
}

// runSteps executes the ordered setup pipeline (drop → create → load → index →
// analyze → optional validate). Returns the first step error.
func (w *workload) runSteps(ctx context.Context, b *bench.Bench) error {
	addStep := func(name string, fn func() error) error { return b.Step(name, fn) }

	if err := addStep("drop_schema", w.dropSchema(ctx, b)); err != nil {
		return err
	}

	if err := addStep("create_schema", w.createSchema(ctx, b)); err != nil {
		return err
	}

	if w.useUnlogged {
		if err := addStep("set_unlogged", w.setUnlogged(ctx, b, "UNLOGGED")); err != nil {
			return err
		}
	}

	if err := addStep("load_data", func() error {
		for _, table := range tpcdsTables {
			if _, err := b.InsertTpcds(ctx, table, w.scaleFactor, w.loadWorkers); err != nil {
				return fmt.Errorf("load %s: %w", table, err)
			}
		}

		return nil
	}); err != nil {
		return err
	}

	if err := addStep("create_indexes", w.runSchemaSection(ctx, b, "create_indexes")); err != nil {
		return err
	}

	if w.useUnlogged {
		if err := addStep("set_logged", w.setUnlogged(ctx, b, "LOGGED")); err != nil {
			return err
		}
	}

	if err := addStep("analyze", w.analyze(ctx, b)); err != nil {
		return err
	}

	// SF=1 baked answer validation (pg/mysql), once. validateAnswers runs inside a tx
	// that applies the schema's set_timeout/preconfigure_db SETs on the pinned conn.
	// Deviation from tpcds.ts: the TS makes the validate pass and the measured pass
	// mutually exclusive; this runs validate in setup and the measured pass in Iterate
	// (mirrors the tpch port), so the queries execute twice at SF=1.
	if !w.throughput && w.genStream < 0 && w.isPgOrMs {
		if err := addStep("validate_answers", func() error {
			validateAnswers(
				ctx, b, w.schemaSQL, w.querySQL, w.querySQL.Names(""),
				w.scaleFactor, w.driver, w.validateForce,
			)

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func (w *workload) Iterate(ctx context.Context, b *bench.Bench) error {
	return b.StepSilent("workload", func() error {
		queries, err := w.resolveQueries(b)
		if err != nil {
			return err
		}
		// The measured pass runs queries raw (no planner SETs), matching tpcds.ts: the
		// set_timeout/preconfigure_db session setup is a validate-pass concern (applied
		// inside the validation tx above) and is unnecessary for the throughput pass.
		lg := b.Logger().Sugar()

		for _, q := range queries {
			start := time.Now()
			err := b.Exec(ctx, q.sql, nil)

			ms := time.Since(start).Milliseconds()
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				b.RecordQueryError(q.name, err)

				continue
			}

			lg.Infof("[tpcds] %s: ok in %dms", q.name, ms)
		}

		return nil
	})
}

func (*workload) Teardown(_ context.Context, b *bench.Bench) error {
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
		streamIdx = int(b.VUID()) - 1 //nolint:gosec // G115: value bounded by scale factor, no overflow path
	} else {
		streamIdx = w.genStream
	}

	return generateStream(string(w.driver), w.scaleFactor, w.seed, streamIdx)
}

// generateStream renders one TPC-DS query stream in-process (port of
// cmd/xk6air.GenerateTpcdsQueries, called here directly to avoid the cmd/xk6air import).
func generateStream(dialect string, scale float64, seed int64, stream int) ([]namedQuery, error) {
	d, ok := dsqgen.DialectByName(dialect)
	if !ok {
		return nil, fmt.Errorf("%w %q", errUnknownDialect, dialect)
	}

	res, err := dsqgen.Generate(d, scale, seed, stream)
	if err != nil {
		return nil, err
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

func (w *workload) runSchemaSection(ctx context.Context, b *bench.Bench, section string) func() error {
	return func() error { return w.runSection(ctx, b, w.schemaSQL, section) }
}

func (w *workload) dropSchema(ctx context.Context, b *bench.Bench) func() error {
	return func() error {
		// ydb/picodata have no CASCADE; drop from the schema file's drop_schema section.
		if w.driver == bench.DriverYDB || w.driver == bench.DriverPicodata {
			return w.runSection(ctx, b, w.schemaSQL, "drop_schema")
		}
		// pg/mysql: reverse load order, CASCADE (mysql accepts/ignores the keyword).
		for i := len(tpcdsTables) - 1; i >= 0; i-- {
			if err := b.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", tpcdsTables[i]), nil); err != nil {
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
		case bench.DriverPostgres:
			return b.Exec(ctx, "ANALYZE", nil)
		case bench.DriverMySQL:
			for _, table := range tpcdsTables {
				if err := b.Exec(ctx, "ANALYZE TABLE "+table, nil); err != nil {
					return fmt.Errorf("analyze %s: %w", table, err)
				}
			}
		case bench.DriverPicodata, bench.DriverYDB, bench.DriverNoop, bench.DriverCSV:
			// no ANALYZE; planner runs on index stats from create_indexes.
		}
		// ydb/picodata: no ANALYZE; planner runs on index stats from create_indexes.
		return nil
	}
}
