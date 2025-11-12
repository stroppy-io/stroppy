package postgres

import (
	"strings"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres/queries"
)

// streamingCopySource implements pgx.CopyFromSource to generate values on-demand
// without loading all rows into memory.
type streamingCopySource struct {
	driver      *Driver
	descriptor  *stroppy.InsertDescriptor
	count       int64
	current     int64
	values      []any
	err         error
	transaction *stroppy.DriverTransaction
	unit        *stroppy.UnitDescriptor
}

func NewStreamingCopySource(
	d *Driver,
	descriptor *stroppy.InsertDescriptor,
	count int64,
) *streamingCopySource {

	return &streamingCopySource{
		driver:  d,
		count:   count,
		current: 0,
		values:  make([]any, strings.Count(queries.BadInsertSQL(descriptor), " ")),
		unit: &stroppy.UnitDescriptor{
			Type: &stroppy.UnitDescriptor_Insert{
				Insert: descriptor,
			},
		},
	}
}

// Next advances to the next row.
func (s *streamingCopySource) Next() bool {
	if s.current >= s.count {
		return false
	}

	// NOTE: known that ctx not used at query generatin
	s.transaction, s.err = s.driver.GenerateNextUnit(nil, s.unit)
	if s.err != nil {
		return false
	}

	s.err = s.driver.fillParamsToValues(s.transaction.GetQueries()[0], s.values)
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
