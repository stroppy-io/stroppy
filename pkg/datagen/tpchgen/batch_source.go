package tpchgen

import (
	"errors"
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/gen"
	"github.com/stroppy-io/stroppy/third_party/gotpc/dbgen"
)

// tpchBytesBudget is the per-row byte budget for every TPC-H text column.
// TPC-H variable-text fields top out at c_comment [1..117]; 200 covers all
// text columns the dbgen project funcs emit (names, addresses, comments,
// formatted dates, status flags).
const tpchBytesBudget = 200

// Sentinel errors for the typed adapter. Wrap with %w so callers can match.
var (
	ErrColumnType  = errors.New("tpchgen: unexpected column type")
	ErrSchemaBuild = errors.New("tpchgen: schema build")
)

// tableKinds maps each TPC-H table name to the gen.Kind of its columns in
// emission order. It mirrors the value types the dbgen project funcs emit:
// int64 for keys/quantities/sizes, float64 for money (dbgen.Money returns
// float64), and bytes for text and formatted dates. MaterializeRow reproduces
// the exact []any the legacy streamSource yields, so driver output is
// byte-identical.
var tableKinds = map[string][]gen.Kind{
	"region": {gen.KindInt64, gen.KindBytes, gen.KindBytes},
	"nation": {gen.KindInt64, gen.KindBytes, gen.KindInt64, gen.KindBytes},
	"part": {
		gen.KindInt64, gen.KindBytes, gen.KindBytes, gen.KindBytes, gen.KindBytes,
		gen.KindInt64, gen.KindBytes, gen.KindFloat64, gen.KindBytes,
	},
	"partsupp": {gen.KindInt64, gen.KindInt64, gen.KindInt64, gen.KindFloat64, gen.KindBytes},
	"supplier": {
		gen.KindInt64, gen.KindBytes, gen.KindBytes, gen.KindInt64,
		gen.KindBytes, gen.KindFloat64, gen.KindBytes,
	},
	"customer": {
		gen.KindInt64, gen.KindBytes, gen.KindBytes, gen.KindInt64,
		gen.KindBytes, gen.KindFloat64, gen.KindBytes, gen.KindBytes,
	},
	"orders": {
		gen.KindInt64, gen.KindInt64, gen.KindBytes, gen.KindFloat64,
		gen.KindBytes, gen.KindBytes, gen.KindBytes, gen.KindInt64, gen.KindBytes,
	},
	"lineitem": {
		gen.KindInt64, gen.KindInt64, gen.KindInt64, gen.KindInt64, gen.KindInt64,
		gen.KindFloat64, gen.KindFloat64, gen.KindFloat64,
		gen.KindBytes, gen.KindBytes, gen.KindBytes, gen.KindBytes,
		gen.KindBytes, gen.KindBytes, gen.KindBytes, gen.KindBytes,
	},
}

// NewBatchSource returns a [gen.BatchSource] over `table` at scale `sf`, the
// typed successor to [New]. It wraps the same dbgen generator-per-partition
// state, canonical seeking, and entity fan-out as the legacy Partitionable;
// the only seam that changes is the contract (BatchSource/Cursor filling a
// typed columnar batch instead of source.RowSource yielding []any).
//
// dbgen's Make* funcs allocate []any per entity internally, so generation
// here is not allocation-free; the typed batch fill itself allocates nothing
// after Prepare. The allocation boundary is therefore dbgen, not the adapter.
// Rewriting dbgen to write into typed columnar storage is out of scope.
func NewBatchSource(table string, sf float64) (gen.BatchSource, error) {
	if sf <= 0 {
		return nil, fmt.Errorf("%w: %g", ErrNonPositiveScale, sf)
	}

	spec, ok := specs[table]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTable, table)
	}

	kinds := tableKinds[table]
	if len(kinds) != len(spec.columns) {
		return nil, fmt.Errorf(
			"%w: kinds/columns mismatch for %q: %d vs %d", ErrSchemaBuild, table, len(kinds), len(spec.columns),
		)
	}

	b := gen.NewSchemaBuilder()
	cols := make([]gen.Column, len(spec.columns))

	for i, name := range spec.columns {
		switch kinds[i] {
		case gen.KindInt64:
			cols[i] = b.Int64(name)
		case gen.KindFloat64:
			cols[i] = b.Float64(name)
		case gen.KindBytes:
			cols[i] = b.Bytes(name, tpchBytesBudget)
		default:
			return nil, fmt.Errorf("%w: unsupported kind %s for %q", ErrSchemaBuild, kinds[i], name)
		}
	}

	return &tpchBatchSource{
		table:  table,
		spec:   spec,
		sf:     sf,
		schema: b.Build(),
		cols:   cols,
		kinds:  kinds,
	}, nil
}

// tpchBatchSource adapts the dbgen generator to gen.BatchSource.
type tpchBatchSource struct {
	table  string
	spec   tableSpec
	sf     float64
	schema gen.Schema
	cols   []gen.Column
	kinds  []gen.Kind
}

func (s *tpchBatchSource) Schema() gen.Schema { return s.schema }

func (s *tpchBatchSource) Units() int64 {
	dbgen.EnsureInit(s.sf)

	return s.spec.entityCount(s.sf)
}

// TotalRows reports the output row count for progress and stats; exact for
// flat tables and partsupp, nominal estimate for lineitem — matching the
// legacy generator.TotalRows.
func (s *tpchBatchSource) TotalRows() int64 {
	dbgen.EnsureInit(s.sf)

	per := max(s.spec.rowsPerEntity, 1)

	return s.spec.entityCount(s.sf) * per
}

// Prepare returns a cursor over entities [start, start+count). It reuses the
// legacy generator.Partition (private dbgen.Generator + seek) and drains it
// into a typed batch. count < 0 means "from start to the end".
func (s *tpchBatchSource) Prepare(start, count int64, batchRows int) (gen.Cursor, error) {
	if start < 0 {
		return nil, fmt.Errorf("tpchgen: negative prepare start %d: %w", start, gen.ErrPrepareRange)
	}

	if batchRows < 1 {
		batchRows = 256
	}

	// Reuse the legacy Partitionable: it builds a private dbgen.Generator,
	// seeks it to start, and returns a streaming RowSource over entities.
	part, err := (&generator{spec: s.spec, sf: s.sf}).Partition(start, count)
	if err != nil {
		return nil, err
	}

	return &tpchCursor{src: s, stream: part, batch: gen.NewBatch(s.schema, batchRows)}, nil
}

// tpchCursor drains the legacy streamSource into a reusable typed batch.
type tpchCursor struct {
	src    *tpchBatchSource
	stream source.RowSource
	batch  *gen.Batch
}

func (c *tpchCursor) Next() (*gen.Batch, error) {
	c.batch.Reset()

	for c.batch.Len() < c.batch.Cap() {
		row, err := c.stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, err
		}

		if err := fillTpchRow(c.batch.AddRow(), row, c.src.cols, c.src.kinds); err != nil {
			return nil, err
		}
	}

	if c.batch.Len() == 0 {
		return nil, io.EOF
	}

	return c.batch, nil
}

// fillTpchRow copies one dbgen []any row into a typed batch row. The kinds
// mirror the project-func value types, so every cell round-trips through
// MaterializeRow to the same any the legacy streamSource emitted.
func fillTpchRow(r gen.Row, row []any, cols []gen.Column, kinds []gen.Kind) error {
	for i, v := range row {
		switch kinds[i] {
		case gen.KindInt64:
			// dbgen project funcs cast every key/quantity to int64 explicitly.
			n, ok := v.(int64)
			if !ok {
				return fmt.Errorf("col %d: expected int64, got %T: %w", i, v, ErrColumnType)
			}

			r.SetInt64(cols[i], n)
		case gen.KindFloat64:
			f, ok := v.(float64)
			if !ok {
				return fmt.Errorf("col %d: expected float64, got %T: %w", i, v, ErrColumnType)
			}

			r.SetFloat64(cols[i], f)
		case gen.KindBytes:
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("col %d: expected string, got %T: %w", i, v, ErrColumnType)
			}

			dst, err := r.Bytes(cols[i], len(s))
			if err != nil {
				return err
			}

			copy(dst, s)
		default:
			return fmt.Errorf("col %d: kind %s: %w", i, kinds[i], ErrColumnType)
		}
	}

	return nil
}
