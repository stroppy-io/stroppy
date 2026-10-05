package gen

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/stroppy-io/stroppy/v6/internal/rowmap"
)

const (
	defaultRowByteBudget = 4096
	defaultRowBatch      = 64
)

// RowsOption bounds storage used by an ordinary struct-row adapter.
type (
	RowsOption  func(*rowsOptions)
	rowsOptions struct {
		bytes int
		batch int
	}
)

// MaxBytes bounds each variable-length column per row; default is 4096.
func MaxBytes(n int) RowsOption {
	if n < 1 {
		panic("gen: byte budget must be positive")
	}

	return func(o *rowsOptions) { o.bytes = n }
}

func BatchRows(n int) RowsOption {
	if n < 1 {
		panic("gen: batch size must be positive")
	}

	return func(o *rowsOptions) { o.batch = n }
}

// FromRows adapts an indexed ordinary struct function into reusable typed batches.
// Callback content must be independent of partitioning. Arbitrary callback allocations
// are permitted; strings/bytes are copied into bounded batch storage.
//
//nolint:gocognit,cyclop,funlen // field kinds map directly to existing batch column storage.
func FromRows[T any](totalRows int64, fn func(uint64) (T, error), options ...RowsOption) *IndexedSource {
	o := rowsOptions{bytes: defaultRowByteBudget, batch: defaultRowBatch}
	for _, option := range options {
		option(&o)
	}

	typ := reflect.TypeFor[T]()

	names, indexes, err := rowmap.Columns(typ)
	if err != nil {
		panic(err)
	}

	builder := NewSchemaBuilder()
	columns := make([]Column, len(names))

	kinds := make([]Kind, len(names))
	for i, name := range names {
		field := typ.Field(indexes[i]).Type
		if field.Kind() == reflect.Pointer {
			field = field.Elem()
		}

		switch {
		case field == reflect.TypeFor[time.Time]():
			columns[i] = builder.Time(name)
			kinds[i] = KindTime
		case field.Kind() == reflect.Bool:
			columns[i] = builder.Bool(name)
			kinds[i] = KindBool
		case field.Kind() == reflect.Float32 || field.Kind() == reflect.Float64:
			columns[i] = builder.Float64(name)
			kinds[i] = KindFloat64
		case field.Kind() >= reflect.Int && field.Kind() <= reflect.Int64:
			columns[i] = builder.Int64(name)
			kinds[i] = KindInt64
		case field.Kind() == reflect.String || (field.Kind() == reflect.Slice && field.Elem().Kind() == reflect.Uint8):
			columns[i] = builder.Bytes(name, o.bytes)
			kinds[i] = KindBytes
		default:
			panic(fmt.Errorf("%w: field %s", errStructSource, field))
		}
	}

	fill := func(row Row, entity uint64) error {
		value, err := fn(entity)
		if err != nil {
			return err
		}

		v := reflect.ValueOf(value)
		for i, index := range indexes {
			field := v.Field(index)
			if field.Kind() == reflect.Pointer {
				if field.IsNil() {
					row.SetNull(columns[i])

					continue
				}

				field = field.Elem()
			}

			switch kinds[i] {
			case KindInt64:
				row.SetInt64(columns[i], field.Int())
			case KindFloat64:
				row.SetFloat64(columns[i], field.Float())
			case KindBool:
				row.SetBool(columns[i], field.Bool())
			case KindTime:
				timestamp, ok := field.Interface().(time.Time)
				if !ok {
					return fmt.Errorf("%w: time column %s", errStructSource, names[i])
				}

				row.SetTime(columns[i], timestamp)
			case KindBytes:
				var data []byte
				if field.Kind() == reflect.String {
					data = []byte(field.String())
				} else {
					data = field.Bytes()
				}

				dst, err := row.Bytes(columns[i], len(data))
				if err != nil {
					return fmt.Errorf("column %s: %w", names[i], err)
				}

				copy(dst, data)
			}
		}

		return nil
	}

	return NewIndexedSource(builder.Build(), Root{}, "", totalRows, o.batch, fill)
}

var errStructSource = errors.New("gen: invalid struct row source")
