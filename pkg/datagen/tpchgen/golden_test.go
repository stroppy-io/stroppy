package tpchgen_test

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"strconv"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/tpchgen"
)

// goldenHashes pins the exact gotpc output at sf=0.01 so refactors (e.g. the
// state-split fork) cannot silently change generated values.
var goldenHashes = map[string]string{
	"region":   "4ed40df53ebd5a36",
	"nation":   "ba4af8be6ab7fe9",
	"part":     "3ec582a597d8bc6a",
	"supplier": "881d6d8e291f82f0",
	"partsupp": "5d13df170072c4a7",
	"customer": "45dec1a9546d3bef",
	"orders":   "b22e5f440f639965",
	"lineitem": "d9c0ff06fb9ae0fb",
}

func TestGoldenHashSF001(t *testing.T) {
	tables := []string{"region", "nation", "part", "supplier", "partsupp", "customer", "orders", "lineitem"}
	for _, table := range tables {
		g, err := tpchgen.New(table, sf)
		if err != nil {
			t.Fatal(err)
		}

		src, err := g.Partition(0, -1)
		if err != nil {
			t.Fatal(err)
		}

		h := fnv.New64a()

		for {
			row, err := src.Next()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				t.Fatal(err)
			}

			fmt.Fprintln(h, row...)
		}

		got := strconv.FormatUint(h.Sum64(), 16)
		if want, ok := goldenHashes[table]; ok {
			if got != want {
				t.Errorf("%s: hash %s != golden %s", table, got, want)
			}
		} else {
			t.Logf("GOLDEN %q: %q,", table, got)
		}
	}
}
