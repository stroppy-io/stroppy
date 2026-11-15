package dsdgen

import (
	"strconv"
)

type rowScalar interface {
	int | int64 | string | Decimal
}

// RowCell keeps borrowed scalar values typed until canonical text is needed.
// Its storage belongs to one stream and is overwritten by the next row.
type RowCell struct {
	integer int64
	text    string
	decimal Decimal
	kind    byte
}

func (c *RowCell) AppendText(dst []byte) []byte {
	switch c.kind {
	case 'i':
		return strconv.AppendInt(dst, c.integer, 10)
	case 's':
		return append(dst, c.text...)
	default:
		return c.decimal.AppendText(dst)
	}
}

type rowValues struct {
	cells [64]RowCell
	next  int
}

func rowValue[T rowScalar](values *rowValues, value T) any {
	if values == nil {
		return value
	}
	cell := &values.cells[values.next]
	values.next++
	switch value := any(value).(type) {
	case int:
		cell.integer, cell.kind = int64(value), 'i'
	case int64:
		cell.integer, cell.kind = value, 'i'
	case string:
		cell.text, cell.kind = value, 's'
	case Decimal:
		cell.decimal, cell.kind = value, 'd'
	}

	return cell
}
