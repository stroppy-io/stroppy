package lookup

import (
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

var lookupRowSink any

func BenchmarkLookupRegistryEvalRow(b *testing.B) {
	attrs := []*dgproto.Attr{
		attr("id", rowIndexExpr()),
		attr("v1", addExpr(rowIndexExpr(), litInt(1))),
		attr("v2", addExpr(rowIndexExpr(), litInt(2))),
		attr("v3", addExpr(rowIndexExpr(), litInt(3))),
		attr("v4", addExpr(rowIndexExpr(), litInt(4))),
		attr("v5", addExpr(rowIndexExpr(), litInt(5))),
		attr("v6", addExpr(rowIndexExpr(), litInt(6))),
		attr("v7", addExpr(rowIndexExpr(), litInt(7))),
	}

	reg, err := NewLookupRegistry([]*dgproto.LookupPop{pop2("bench", 1_000_000, attrs)}, nil, 10)
	if err != nil {
		b.Fatal(err)
	}

	population := reg.pops["bench"]

	b.ReportAllocs()

	for i := range b.N {
		row, rowErr := reg.evalRow(population, int64(i))
		if rowErr != nil {
			b.Fatal(rowErr)
		}

		lookupRowSink = row
	}
}
