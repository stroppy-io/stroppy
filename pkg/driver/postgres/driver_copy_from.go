package postgres

import (
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
)

// streamingCopySource implements pgx.CopyFromSource to generate values on-demand
// without loading all rows into memory.
type streamingCopySource struct {
	leftCount int32
	values    []any
	err       error
	builder   *queries.QueryBuilder
}

func newStreamingCopySource(
	builder *queries.QueryBuilder,
) *streamingCopySource {
	return &streamingCopySource{
		leftCount: builder.Count(),
		values:    make([]any, len(builder.Columns())),
		builder:   builder,
	}
}

// Next advances to the next row.
func (s *streamingCopySource) Next() bool {
	if s.leftCount == 0 {
		return false
	}

	s.err = s.builder.Build(s.values)
	if s.err != nil {
		return false
	}

	s.leftCount--

	return true
}

// Values returns the values for the current row.
func (s *streamingCopySource) Values() ([]any, error) { return s.values, s.err }

// Err returns any error that occurred during iteration.
func (s *streamingCopySource) Err() error { return s.err }
