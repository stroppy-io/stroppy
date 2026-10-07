package dsdgen

// GenerateUniformRandomDecimal returns a uniform decimal in [min, max]. The
// precision is the smaller of the two operands' precisions and the draw uses the
// full 64-bit RNG value (no int32 truncation, unlike the integer variant).
// Mirrors generateUniformRandomDecimal.
func GenerateUniformRandomDecimal(min, max Decimal, s *RNStream) Decimal {
	precision := min.Precision
	if max.Precision < precision {
		precision = max.Precision
	}

	number := s.NextRandom()
	number %= max.Number - min.Number + 1
	number += min.Number

	return Decimal{Precision: precision, Number: number}
}

// GenerateUniformRandomDate returns a uniform date in [min, max] inclusive.
// Mirrors generateUniformRandomDate.
func GenerateUniformRandomDate(min, max Date, s *RNStream) Date {
	span := ToJulianDays(max) - ToJulianDays(min)

	return FromJulianDays(ToJulianDays(min) + GenerateUniformRandomInt(0, span, s))
}
