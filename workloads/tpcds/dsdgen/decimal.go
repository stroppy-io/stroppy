package dsdgen

import (
	"fmt"
	"strconv"
	"strings"
)

// Decimal is dsdgen's fixed-point money/number type. Per the reference, the
// names are reversed relative to SQL: precision is the number of fractional
// digits, and number is the unscaled integer value. Arithmetic is intentionally
// bug-compatible with decimal.c (e.g. add/subtract ignore differing precision).
type Decimal struct {
	Precision int
	Number    int64
}

// Common decimal constants from Decimal.java.
var (
	DecimalZero       = Decimal{2, 0}
	DecimalOneHalf    = Decimal{2, 50}
	DecimalNinePct    = Decimal{2, 9}
	DecimalOneHundred = Decimal{2, 10000}
	DecimalOne        = Decimal{2, 100}
)

// ParseDecimal parses a decimal literal, preserving the number of fractional
// digits as the precision.
func ParseDecimal(s string) Decimal {
	if dot := strings.IndexByte(s, '.'); dot == -1 {
		n, _ := strconv.Atoi(s)

		return Decimal{Precision: 0, Number: int64(n)}
	} else {
		fractional := s[dot+1:]
		n, _ := strconv.Atoi(s[:dot] + fractional)

		return Decimal{Precision: len(fractional), Number: int64(n)}
	}
}

func maxPrecision(a, b Decimal) int {
	if a.Precision > b.Precision {
		return a.Precision
	}

	return b.Precision
}

// AddDecimal adds two decimals (C-compatible: precision is the max, numbers are
// added directly even when precisions differ).
func AddDecimal(a, b Decimal) Decimal {
	return Decimal{Precision: maxPrecision(a, b), Number: a.Number + b.Number}
}

// SubDecimal subtracts b from a, mirroring the C code.
func SubDecimal(a, b Decimal) Decimal {
	return Decimal{Precision: maxPrecision(a, b), Number: a.Number - b.Number}
}

// MulDecimal multiplies two decimals, rounding the scaled product down.
func MulDecimal(a, b Decimal) Decimal {
	precision := maxPrecision(a, b)
	number := a.Number * b.Number
	for i := a.Precision + b.Precision; i > precision; i-- {
		number /= 10
	}

	return Decimal{Precision: precision, Number: number}
}

// DivDecimal divides a by b using float32 math, exactly as divide() in the C
// port does (the float width matters for byte-exact results).
func DivDecimal(a, b Decimal) Decimal {
	precision := maxPrecision(a, b)

	f1 := float32(a.Number)
	for i := a.Precision; i < precision; i++ {
		f1 *= 10.0
	}
	for i := 0; i < precision; i++ {
		f1 *= 10.0
	}

	f2 := float32(b.Number)
	for i := b.Precision; i < precision; i++ {
		f2 *= 10.0
	}

	return Decimal{Precision: precision, Number: int64(int32(f1 / f2))}
}

// NegateDecimal returns -d.
func NegateDecimal(d Decimal) Decimal { return Decimal{Precision: d.Precision, Number: -d.Number} }

// DecimalFromInteger wraps an int as a precision-0 decimal.
func DecimalFromInteger(n int) Decimal { return Decimal{Precision: 0, Number: int64(n)} }

// String renders the decimal the way print_decimal in print.c does: divide the
// unscaled number by 10^precision in floating point, then format with the fixed
// number of fractional digits.
func (d Decimal) String() string {
	temp := float64(d.Number)
	for i := 0; i < d.Precision; i++ {
		temp /= 10.0
	}

	return fmt.Sprintf("%.*f", d.Precision, temp)
}
