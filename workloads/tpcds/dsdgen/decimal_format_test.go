package dsdgen

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecimalAppendTextMatchesCanonical(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(10, 20))
	for precision := 0; precision <= 9; precision++ {
		for _, number := range []int64{0, 1, -1, (1 << 40) - 1, 1 << 40, -(1 << 40), (1 << 40) + 1, math.MaxInt64, math.MinInt64} {
			value := Decimal{Number: number, Precision: precision}
			require.Equal(t, value.String(), string(value.AppendText(nil)), value)
		}
		for range 10000 {
			value := Decimal{Number: rng.Int64N(1<<41) - (1 << 40), Precision: precision}
			require.Equal(t, value.String(), string(value.AppendText(nil)), value)
		}
	}
}
