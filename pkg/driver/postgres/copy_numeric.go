package postgres

import (
	"encoding/binary"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	copyNumericMaxText     = 64
	copyNumericGroupDigits = 4
	copyNumericDecimalBase = 10
)

type copyNumericCodec struct{ pgtype.Codec }

func (c copyNumericCodec) PlanEncode(m *pgtype.Map, oid uint32, format int16, value any) pgtype.EncodePlan {
	plan := c.Codec.PlanEncode(m, oid, format, value)
	if format == pgtype.BinaryFormatCode && plan != nil {
		switch value.(type) {
		case copyText, *copyText, *copyBytes:
			return copyNumericPlan{fallback: plan}
		}
	}

	return plan
}

type copyNumericPlan struct{ fallback pgtype.EncodePlan }

func (p copyNumericPlan) Encode(value any, buf []byte) ([]byte, error) {
	var text copyText

	switch value := value.(type) {
	case *copyText:
		if value != nil {
			text = *value
		}
	case *copyBytes:
		if value != nil {
			text = copyText(value.data)
		}
	case copyText:
		text = value
	}

	if encoded, supported := appendCopyNumeric(buf, string(text)); supported {
		return encoded, nil
	}

	return p.fallback.Encode(value, buf)
}

func copyNumericParts(text string) (whole, fraction string, sign uint16, ok bool) {
	if text == "" || len(text) > copyNumericMaxText {
		return "", "", 0, false
	}

	if text[0] == '-' || text[0] == '+' {
		if text[0] == '-' {
			sign = 0x4000
		}

		text = text[1:]
	}

	whole, fraction, _ = strings.Cut(text, ".")
	if whole == "" && fraction == "" {
		return "", "", 0, false
	}

	for _, part := range []string{whole, fraction} {
		for i := range len(part) {
			if part[i] < '0' || part[i] > '9' {
				return "", "", 0, false
			}
		}
	}

	return whole, fraction, sign, true
}

// Small decimal literals need only base-10000 digits, not arbitrary-precision
// arithmetic. Other numeric syntax retains the connection's original codec.
func appendCopyNumeric(buf []byte, text string) ([]byte, bool) {
	whole, fraction, sign, ok := copyNumericParts(text)
	if !ok {
		return buf, false
	}

	var storage [copyNumericMaxText/copyNumericGroupDigits + 1]uint16

	groups := storage[:0]

	var group uint16
	for i := range len(whole) {
		group = group*copyNumericDecimalBase + uint16(whole[i]-'0')
		if (len(whole)-i-1)%copyNumericGroupDigits == 0 {
			groups = append(groups, group)
			group = 0
		}
	}

	weight := len(groups) - 1
	for i := 0; i < len(fraction); i += copyNumericGroupDigits {
		for j := range copyNumericGroupDigits {
			group *= copyNumericDecimalBase
			if i+j < len(fraction) {
				group += uint16(fraction[i+j] - '0')
			}
		}

		groups = append(groups, group)
		group = 0
	}

	for len(groups) > 0 && groups[0] == 0 {
		groups = groups[1:]
		weight--
	}

	for len(groups) > 0 && groups[len(groups)-1] == 0 {
		groups = groups[:len(groups)-1]
	}

	if len(groups) == 0 {
		weight, sign = 0, 0
		if fraction != "" {
			// pgx v5.13.0's binary decoder discards scale when ndigits is zero.
			storage[0], groups, weight = 0, storage[:1], -1
		}
	}

	buf = binary.BigEndian.AppendUint16(buf, uint16(len(groups)))   //nolint:gosec // Bounded to 17 groups.
	buf = binary.BigEndian.AppendUint16(buf, uint16(int16(weight))) //nolint:gosec // Bounded signed wire weight.
	buf = binary.BigEndian.AppendUint16(buf, sign)

	buf = binary.BigEndian.AppendUint16(buf, uint16(len(fraction))) //nolint:gosec // Bounded to 63 digits.
	for _, digit := range groups {
		buf = binary.BigEndian.AppendUint16(buf, digit)
	}

	return buf, true
}
