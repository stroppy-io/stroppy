package expr_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// These tests anchor CLAUDE.md §6: the tx-time Draw path (xk6air
// iter 2) MUST compose its PRNG through the same seed.Derive
// invocation the full evaluator uses for (streamID=0,
// attrPath="draw", rowIdx=key). They live next to the kernels
// because the xk6air package cannot be imported here (separate
// module, internal/common boundary).

// drawEquivRoot is the single root seed shared by every equivalence
// case below. Keeping it hoisted to package scope lets the helpers
// drop an otherwise always-constant parameter.
const drawEquivRoot uint64 = 0xA3_5F_EE_10_BE_EF_CA_FE

// drawPRNG recreates the seed composition iter-2's *drawX structs use
// in draw_arms.go. If it drifts from evalContext.Draw, this test
// catches it before the drawbench numbers do.
func drawPRNG(key int64) *rand.Rand {
	k := seed.Derive(drawEquivRoot, "draw", "s0", strconv.FormatInt(key, 10))

	return seed.PRNG(k)
}

// evalContextPRNG mirrors the composition in runtime.evalContext.Draw.
// The equivalence suite always compares against the canonical evaluator
// path — attrPath="draw", streamID=0 — so both are fixed here. Keeping
// this helper and drawPRNG in one file makes divergences stand out in
// a single diff.
func evalContextPRNG(rowIdx int64) *rand.Rand {
	return seed.PRNG(seed.Derive(
		drawEquivRoot,
		"draw",
		"s0",
		strconv.FormatInt(rowIdx, 10),
	))
}

func TestDraw2_SeedCompositionMatchesEvaluator(t *testing.T) {
	t.Parallel()

	for _, key := range []int64{0, 1, 7, 42, 1_000_000} {
		drawRand := drawPRNG(key)
		evalRand := evalContextPRNG(key)

		for i := range 8 {
			require.Equalf(t, evalRand.Uint64(), drawRand.Uint64(),
				"iter-2 seed diverged from evaluator at key=%d i=%d", key, i)
		}
	}
}

func TestDraw2_IntUniformMatchesEvaluator(t *testing.T) {
	t.Parallel()

	for _, key := range []int64{0, 5, 99, 12345} {
		draw := drawPRNG(key)
		eval := evalContextPRNG(key)

		v1, err := expr.KernelIntUniform(draw, 1, 1_000_000)
		require.NoError(t, err)

		v2, err := expr.KernelIntUniform(eval, 1, 1_000_000)
		require.NoError(t, err)

		require.Equalf(t, v2, v1, "int_uniform divergence at key=%d", key)
	}
}

func TestDraw2_NURandMatchesEvaluator(t *testing.T) {
	t.Parallel()

	for _, key := range []int64{0, 11, 555} {
		v1, err := expr.KernelNURand(drawPRNG(key), 255, 0, 9999, 0)
		require.NoError(t, err)

		v2, err := expr.KernelNURand(evalContextPRNG(key), 255, 0, 9999, 0)
		require.NoError(t, err)

		require.Equal(t, v2, v1)
	}
}

func TestDraw2_ASCIIMatchesEvaluator(t *testing.T) {
	t.Parallel()

	alphabet := []*dgproto.AsciiRange{{Min: 0x61, Max: 0x7A}}

	for _, key := range []int64{0, 2, 99} {
		v1, err := expr.KernelASCII(drawPRNG(key), 3, 10, alphabet)
		require.NoError(t, err)

		v2, err := expr.KernelASCII(evalContextPRNG(key), 3, 10, alphabet)
		require.NoError(t, err)

		require.Equal(t, v2, v1)
	}
}

func TestDraw2_DictMatchesEvaluator(t *testing.T) {
	t.Parallel()

	dict := &dgproto.Dict{
		Columns: []string{"name"},
		Rows: []*dgproto.DictRow{
			{Values: []string{"alpha"}},
			{Values: []string{"beta"}},
			{Values: []string{"gamma"}},
			{Values: []string{"delta"}},
		},
	}

	for _, key := range []int64{0, 3, 50} {
		v1, err := expr.KernelDict(drawPRNG(key), dict, "")
		require.NoError(t, err)

		v2, err := expr.KernelDict(evalContextPRNG(key), dict, "")
		require.NoError(t, err)

		require.Equal(t, v2, v1)
	}
}

// TestDraw2_PooledPRNGMatchesFresh proves the PCG pooling scheme used
// by acquirePRNG (in xk6air/draw_prng_pool.go) does not perturb
// outputs: seed.SeedPCG in-place on a reusable source yields the same
// stream as seed.PRNG on a fresh one. This is the only assertion that
// guards the "zero-alloc PCG reuse" optimization from silently
// drifting.
func TestDraw2_PooledPRNGMatchesFresh(t *testing.T) {
	t.Parallel()

	for _, key := range []int64{0, 1, 99} {
		k := seed.Derive(drawEquivRoot, "draw", "s0", strconv.FormatInt(key, 10))

		fresh := seed.PRNG(k)

		src := &rand.PCG{}
		seed.SeedPCG(src, k)
		reused := rand.New(src)

		for range 16 {
			require.Equal(t, fresh.Uint64(), reused.Uint64())
		}
	}
}
