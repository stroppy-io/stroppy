package runtime

import (
	"errors"
	"io"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

var runtimeRowSink []any

func BenchmarkRuntimeNextSlotFastPath(b *testing.B) {
	dicts := map[string]*dgproto.Dict{
		"status": {
			Rows: []*dgproto.DictRow{
				{Values: []string{"new"}},
				{Values: []string{"paid"}},
				{Values: []string{"shipped"}},
				{Values: []string{"closed"}},
			},
		},
	}

	attrs := []*dgproto.Attr{
		attr("id", rowIndex()),
		attr("plus_one", binOp(dgproto.BinOp_ADD, col("id"), lit(int64(1)))),
		attr("times_two", binOp(dgproto.BinOp_MUL, col("plus_one"), lit(int64(2)))),
		attr("is_large", binOp(dgproto.BinOp_GT, col("times_two"), lit(int64(1000)))),
		attr("bucket", ifExpr(col("is_large"), lit(int64(1)), lit(int64(0)))),
		attr("status", dictAt("status", col("id"))),
		attr("created_at", callExpr("std.daysToDate", binOp(dgproto.BinOp_ADD, lit(int64(10_000)), col("id")))),
	}

	rt, err := NewRuntime(spec(1_000_000_000, []string{
		"id", "plus_one", "times_two", "is_large", "bucket", "status", "created_at",
	}, attrs, dicts))
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for range b.N {
		row, nextErr := rt.Next()
		if errors.Is(nextErr, io.EOF) {
			b.Fatal("unexpected EOF")
		}

		if nextErr != nil {
			b.Fatal(nextErr)
		}

		runtimeRowSink = row
	}
}
