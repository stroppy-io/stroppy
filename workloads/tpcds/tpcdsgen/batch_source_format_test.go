package tpcdsgen

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/dsdgen"
)

func TestAppendTpcdsCellMatchesCanonicalText(t *testing.T) {
	t.Parallel()

	values := []any{
		"", "000123", int(0), int(-123), int64(math.MaxInt64), int64(math.MinInt64),
		float64(0), math.Copysign(0, -1), 1.25, 1e20, 1e-12,
		dsdgen.Date{Year: 2000, Month: 2, Day: 29},
		dsdgen.Decimal{Number: 0, Precision: 2},
		dsdgen.Decimal{Number: -1, Precision: 2},
		dsdgen.Decimal{Number: math.MaxInt64, Precision: 2},
		dsdgen.Decimal{Number: math.MinInt64, Precision: 4},
	}
	for _, value := range values {
		var buf [tpcdsBytesBudget]byte
		require.Equal(t, fmt.Sprint(value), string(appendTpcdsCell(buf[:0], value)))
	}
}
