package postgres

import (
	"fmt"
	"strings"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
)

// streamingCopySource implements pgx.CopyFromSource to generate values on-demand
// without loading all rows into memory.
type streamingCopySource struct {
	driver  *Driver
	count   int64
	current int64
	values  []any
	err     error
	builder QueryBuilder
}

func newStreamingCopySource(
	d *Driver,
	descriptor *stroppy.InsertDescriptor,
) (*streamingCopySource, error) {
	builder, err := queries.NewQueryBuilder(d.logger, 0, descriptor)
	if err != nil {
		return nil, fmt.Errorf("can't create query builder due to: %w", err)
	}

	return &streamingCopySource{
		driver:  d,
		count:   int64(descriptor.GetCount()),
		current: 0,
		values:  make([]any, strings.Count(queries.BadInsertSQL(descriptor), " ")),
		builder: builder,
	}, nil
}

// Next advances to the next row.
func (s *streamingCopySource) Next() bool {
	if s.current >= s.count {
		return false
	}

	_, s.values, s.err = s.builder.Build()
	if s.err != nil {
		return false
	}

	s.current++

	return true
}

// Values returns the values for the current row.
func (s *streamingCopySource) Values() ([]any, error) {
	return s.values, nil
}

// Err returns any error that occurred during iteration.
func (s *streamingCopySource) Err() error {
	return s.err
}
