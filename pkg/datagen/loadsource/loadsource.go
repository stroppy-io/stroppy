// Package loadsource builds the right source.Partitionable for an InsertSpec
// and adapts the native runtime.Runtime to the source contract. It is the one
// place that knows which generator backend a spec selects, so drivers depend
// only on the source interfaces.
package loadsource

import (
	"fmt"
	"io"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
	"github.com/stroppy-io/stroppy/pkg/datagen/source"
	"github.com/stroppy-io/stroppy/pkg/datagen/tpcdsgen"
	"github.com/stroppy-io/stroppy/pkg/datagen/tpchgen"
)

// Build returns the Partitionable that produces the rows for spec. It selects
// the generator backend from the spec's `generator` oneof: the ported TPC-H
// dbgen generator when `tpch` is set, the ported TPC-DS dsdgen generator when
// `tpcds` is set, otherwise the native seekable runtime.
func Build(spec *dgproto.InsertSpec) (source.Partitionable, error) {
	if spec == nil {
		return nil, fmt.Errorf("loadsource: %w", runtime.ErrInvalidSpec)
	}

	if tpch := spec.GetTpch(); tpch != nil {
		p, err := tpchgen.New(tpch.GetTable(), tpch.GetScaleFactor())
		if err != nil {
			return nil, fmt.Errorf("loadsource: build tpch source: %w", err)
		}

		return p, nil
	}

	if tpcds := spec.GetTpcds(); tpcds != nil {
		p, err := tpcdsgen.New(tpcds.GetTable(), tpcds.GetScaleFactor())
		if err != nil {
			return nil, fmt.Errorf("loadsource: build tpcds source: %w", err)
		}

		return p, nil
	}

	rt, err := runtime.NewRuntime(spec)
	if err != nil {
		return nil, fmt.Errorf("loadsource: build runtime: %w", err)
	}

	return &runtimeSource{rt: rt}, nil
}

// runtimeSource adapts *runtime.Runtime to source.Partitionable. The seed
// runtime is read-only after construction; each Partition clones it and seeks,
// so workers never contend.
type runtimeSource struct{ rt *runtime.Runtime }

// Units == TotalRows for the native runtime: a unit is one output row.
func (s *runtimeSource) Units() int64     { return s.rt.TotalRows() }
func (s *runtimeSource) TotalRows() int64 { return s.rt.TotalRows() }

func (s *runtimeSource) Partition(start, count int64) (source.RowSource, error) {
	clone := s.rt.Clone()
	if err := clone.SeekRow(start); err != nil {
		return nil, fmt.Errorf("loadsource: seek to %d: %w", start, err)
	}

	return &limitSource{rt: clone, remaining: count, unbounded: count < 0}, nil
}

// limitSource wraps a seeked runtime clone and stops after `remaining` rows so
// each partition emits exactly its chunk. A negative count makes it unbounded
// (drain to the runtime's own EOF).
type limitSource struct {
	rt        *runtime.Runtime
	remaining int64
	unbounded bool
}

func (l *limitSource) Columns() []string { return l.rt.Columns() }

func (l *limitSource) Next() ([]any, error) {
	if !l.unbounded && l.remaining <= 0 {
		return nil, io.EOF
	}

	row, err := l.rt.Next()
	if err != nil {
		return nil, err
	}

	l.remaining--

	return row, nil
}
