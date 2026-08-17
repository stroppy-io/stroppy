package gen_test

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/gen"
)

// Example models three columns of a TPC-C-style item row with plain imperative
// Go. The formula reads top-to-bottom with no seed plumbing, no RNG object,
// and no positional column index: field namespaces are derived once, scalars
// are pure one-liners, and variable-length text is drawn and filled into a
// caller-owned buffer in one call.
func Example() {
	root := gen.New(0xC0FFEE)
	items := root.Domain("tpcc/item@1")
	imID := items.Field("i_im_id")
	name := items.Field("i_name")
	price := items.Field("i_price")

	var buf [24]byte

	for _, id := range []uint64{0, 1, 2} {
		im := imID.Int64(id, 1, 10000)
		nd := name.At(id)
		n := gen.AlphaNumeric.Bytes(&nd, buf[:], 14, 24)
		pr := price.Decimal(id, 1, 100, 2)
		fmt.Printf("id=%d im_id=%d name=%q price=%.2f\n", id+1, im, buf[:n], pr)
	}
	// Output:
	// id=1 im_id=4539 name="dgHhw9Gvmr6DowPtD" price=40.03
	// id=2 im_id=4504 name="abMjYAOwfPIrKkiHoP" price=74.11
	// id=3 im_id=3428 name="GHx2EYJZBrRA1CUpqEO" price=90.34
}
