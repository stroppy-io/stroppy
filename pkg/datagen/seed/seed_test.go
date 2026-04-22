package seed_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

func TestSplitMix64(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   uint64
		want uint64
	}{
		{"zero", 0x0000000000000000, 0xE220A8397B1DCDAF},
		{"one", 0x0000000000000001, 0x910A2DEC89025CC1},
		{"deadbeef", 0x00000000DEADBEEF, 0x4ADFB90F68C9EB9B},
		{"max", math.MaxUint64, 0xE4D971771B652C20},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := seed.SplitMix64(tc.in)
			require.Equalf(t, tc.want, got, "SplitMix64(0x%016X)", tc.in)
		})
	}
}

func TestFNV1a64(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want uint64
	}{
		{"empty", "", 0xCBF29CE484222325},
		{"a", "a", 0xAF63DC4C8601EC8C},
		{"ab", "a/b", 0xE620C3190468CF61},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := seed.FNV1a64(tc.in)
			require.Equalf(t, tc.want, got, "FNV1a64(%q)", tc.in)
		})
	}

	// Cross-check: Derive must equal SplitMix64(root ^ FNV1a64(joined)).
	require.Equal(t, seed.SplitMix64(42^seed.FNV1a64("a/b")), seed.Derive(42, "a", "b"))
}

func TestDerive(t *testing.T) {
	t.Parallel()

	t.Run("determinism", func(t *testing.T) {
		t.Parallel()

		a := seed.Derive(42, "orders", "o_custkey")
		b := seed.Derive(42, "orders", "o_custkey")
		require.Equal(t, a, b)
	})

	t.Run("path_order_matters", func(t *testing.T) {
		t.Parallel()

		ab := seed.Derive(0, "a", "b")
		ba := seed.Derive(0, "b", "a")
		require.NotEqual(t, ab, ba)
	})

	t.Run("empty_path_is_splitmix_of_root", func(t *testing.T) {
		t.Parallel()

		// fnv1a64("") == offset basis, so root=0 gives splitmix64(offset_basis).
		const fnvEmptyXorZero = 0xCBF29CE484222325
		require.Equal(t, seed.SplitMix64(fnvEmptyXorZero), seed.Derive(0))
		require.Equal(t, seed.SplitMix64(123^fnvEmptyXorZero), seed.Derive(123))
	})

	t.Run("join_with_slash", func(t *testing.T) {
		t.Parallel()

		// The path elements are joined with "/", so ("a","b") must equal ("a/b").
		require.Equal(t, seed.Derive(0, "a/b"), seed.Derive(0, "a", "b"))
	})

	t.Run("multi_element_paths", func(t *testing.T) {
		t.Parallel()

		seen := make(map[uint64]string)

		for _, p := range [][]string{
			{"lineitem", "l_partkey", "block", "0"},
			{"lineitem", "l_partkey", "block", "1"},
			{"lineitem", "l_suppkey", "block", "0"},
			{"customer", "c_name"},
			{"customer", "c_name", ""},
		} {
			k := seed.Derive(7, p...)
			if prev, ok := seen[k]; ok {
				t.Fatalf("collision: %v vs %s", p, prev)
			}

			seen[k] = p[0]
		}
	})

	t.Run("extreme_roots", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name string
			root uint64
			path []string
			want uint64
		}{
			{"root_zero_empty_path", 0, nil, 0xC3817C016BA4FF30},
			{"root_zero_ab", 0, []string{"a", "b"}, 0x569039D1F57486EA},
			{"root_max_x", math.MaxUint64, []string{"x"}, 0xAC9867BF22F6B11F},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				require.Equal(t, tc.want, seed.Derive(tc.root, tc.path...))
			})
		}
	})
}

func TestPRNG(t *testing.T) {
	t.Parallel()

	const draws = 16

	draw := func(key uint64) [draws]int64 {
		r := seed.PRNG(key)

		var out [draws]int64
		for i := range out {
			out[i] = r.Int64()
		}

		return out
	}

	t.Run("determinism", func(t *testing.T) {
		t.Parallel()

		for _, key := range []uint64{0, 0xDEADBEEF, math.MaxUint64} {
			first := draw(key)
			second := draw(key)
			require.Equalf(t, first, second, "PRNG(%d) must be deterministic", key)
		}
	})

	t.Run("different_keys_diverge", func(t *testing.T) {
		t.Parallel()

		keys := []uint64{0, 1, 2, 0xDEADBEEF, math.MaxUint64}
		seen := make(map[[draws]int64]uint64, len(keys))

		for _, k := range keys {
			seq := draw(k)
			if prev, ok := seen[seq]; ok {
				t.Fatalf("keys %d and %d produced identical sequences", prev, k)
			}

			seen[seq] = k
		}
	})
}
