package seed_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

func TestReusablePRNGMatchesFresh(t *testing.T) {
	t.Parallel()

	for _, key := range []uint64{0, 1, 42, 0xBEEF} {
		fresh := seed.PRNG(key)

		reuse := seed.NewReusablePRNG()
		reuse.Seed(key)

		for range 16 {
			require.Equal(t, fresh.Uint64(), reuse.Rand().Uint64())
		}
	}
}

func TestPooledPRNGMatchesFresh(t *testing.T) {
	t.Parallel()

	for _, key := range []uint64{0, 7, 99} {
		fresh := seed.PRNG(key)

		pooled := seed.AcquirePooledPRNG(key)

		for range 16 {
			require.Equal(t, fresh.Uint64(), pooled.R.Uint64())
		}

		seed.ReleasePooledPRNG(pooled)
	}
}

func TestPooledPRNGReseed(t *testing.T) {
	t.Parallel()

	pooled := seed.AcquirePooledPRNG(1)
	first := pooled.R.Uint64()
	seed.ReleasePooledPRNG(pooled)

	pooled = seed.AcquirePooledPRNG(2)
	require.NotEqual(t, first, pooled.R.Uint64())
	seed.ReleasePooledPRNG(pooled)
}

func TestReusablePRNGReseed(t *testing.T) {
	t.Parallel()

	reuse := seed.NewReusablePRNG()
	reuse.Seed(1)
	first := reuse.Rand().Uint64()

	reuse.Seed(2)
	require.NotEqual(t, first, reuse.Rand().Uint64())
}
