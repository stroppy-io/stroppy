// Package tpcc is the Go-native port of workloads/tpcc/tx.ts: the five TPC-C
// transactions as ordered DML steps inside driver transactions, with the standard
// 45/43/4/4/4 mix. Load/config/prepare are shared structure ported from
// tpcc_common.ts. Initial port covers pg + mysql; picodata/ydb dialect branches
// (OFFSET, list params) are deferred.
package tpcc

import (
	"math/rand/v2"

	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// tpccSyllables + cLastDict: spec §4.3.2.3 — C_LAST is a 3-syllable concat indexed
// by the three base-10 digits of i ∈ [0,999]. 1000 deterministic last names, shared
// by the load and the by-name lookup branches of payment/order_status.
var tpccSyllables = [10]string{"BAR", "OUGHT", "ABLE", "PRI", "PRES", "ESE", "ANTI", "CALLY", "ATION", "EING"}

var cLastDict [1000]string

func init() {
	for i := range 1000 {
		cLastDict[i] = tpccSyllables[i/100] + tpccSyllables[(i/10)%10] + tpccSyllables[i%10]
	}
}

// cLast returns the syllable-concat surname for a NURand index in [0,999].
func cLast(idx int) string { return cLastDict[idx%1000] }

// nurand is the TPC-C §2.1.6 non-uniform random draw, ported from
// pkg/datagen/expr/kernels.go KernelNURand. Used at tx time for c_id / ol_i_id /
// c_last picks. cSalt personalizes paramC per generator so streams stay independent.
func nurand(r *rand.Rand, paramA, lower, upper int, cSalt uint64) int {
	span := int64(upper - lower + 1)
	paramC := int64(seed.SplitMix64(cSalt)) & int64(paramA) //nolint:gosec // G115: scale-bound, no overflow
	aDraw := r.IntN(paramA + 1)
	yDraw := r.IntN(upper-lower+1) + lower

	return int(((int64(aDraw)|int64(yDraw))+paramC)%span) + lower
}

// weightedPick mirrors k6/x/stroppy NewPicker.PickWeighted: cumulative-threshold
// draw over weights.
func weightedPick(r *rand.Rand, weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}

	threshold := r.Float64() * float64(total)

	cum := 0.0
	for i, w := range weights {
		cum += float64(w)
		if cum >= threshold {
			return i
		}
	}

	return len(weights) - 1
}
