package gen

import (
	"testing"
	"time"
)

// TestMaterializeRow writes one row of each kind and a null, then reads
// it back through MaterializeRow into a []any. Verifies the kind→Go-type
// mapping drivers rely on: scalars by value, bytes as string, nulls as
// untyped nil.
func TestMaterializeRow(t *testing.T) {
	t.Parallel()

	schema := NewSchemaBuilder()
	idCol := schema.Int64("i")
	fCol := schema.Float64("f")
	bCol := schema.Bool("b")
	tsCol := schema.Time("ts")
	sCol := schema.Bytes("s", 8)
	built := schema.Build()

	b := NewBatch(built, 2)

	r0 := b.Row(0)
	r0.SetInt64(idCol, 42)
	r0.SetFloat64(fCol, 1.5)
	r0.SetBool(bCol, true)
	r0.SetTime(tsCol, time.Unix(1700000000, 0).UTC())

	dst, err := r0.Bytes(sCol, 5)
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	copy(dst, "hello")

	// Row 1: exercise NULL on a scalar and a bytes column.
	r1 := b.Row(1)
	r1.SetInt64(idCol, 7)
	r1.SetNull(fCol) // float null
	r1.SetBool(bCol, false)
	r1.SetTime(tsCol, time.Unix(0, 0).UTC())
	r1.SetNull(sCol) // bytes null

	b.n = 2

	out := make([]any, 5)
	b.MaterializeRow(0, out)

	if got := out[0].(int64); got != 42 {
		t.Fatalf("i = %v, want 42", got)
	}

	if got := out[1].(float64); got != 1.5 {
		t.Fatalf("f = %v, want 1.5", got)
	}

	if got := out[2].(bool); !got {
		t.Fatalf("b = %v, want true", got)
	}

	if got := out[3].(time.Time); !got.Equal(time.Unix(1700000000, 0).UTC()) {
		t.Fatalf("ts = %v, want 2023-11-14", got)
	}

	if got := out[4].(string); got != "hello" {
		t.Fatalf("s = %q, want hello", got)
	}

	b.MaterializeRow(1, out)

	if out[0].(int64) != 7 {
		t.Fatalf("row1 i = %v, want 7", out[0])
	}

	if out[1] != nil {
		t.Fatalf("row1 f = %v, want nil", out[1])
	}

	if out[4] != nil {
		t.Fatalf("row1 s = %v, want nil", out[4])
	}
}

// TestMaterializeRowBytesStringCopy verifies the bytes materialization
// returns a fresh string that does not alias the batch slab, so the
// caller may hold it across the next batch refill.
func TestMaterializeRowBytesStringCopy(t *testing.T) {
	t.Parallel()

	schema2 := NewSchemaBuilder()
	s2Col := schema2.Bytes("s", 4)
	built2 := schema2.Build()
	b := NewBatch(built2, 1)

	r := b.Row(0)

	dst, err := r.Bytes(s2Col, 3)
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	copy(dst, "abc")

	b.n = 1

	out := make([]any, 1)
	b.MaterializeRow(0, out)

	first := out[0].(string)
	if first != "abc" {
		t.Fatalf("first = %q, want abc", first)
	}

	// Refill the batch with different content; the earlier string must
	// be unaffected (no slab aliasing).
	b.Reset()

	r2 := b.Row(0)

	dst2, err := r2.Bytes(s2Col, 3)
	if err != nil {
		t.Fatalf("Bytes refill: %v", err)
	}

	copy(dst2, "XYZ")

	b.n = 1
	b.MaterializeRow(0, out)

	if out[0].(string) != "XYZ" {
		t.Fatalf("refill = %q, want XYZ", out[0])
	}

	if first != "abc" {
		t.Fatalf("first string mutated after refill: %q", first)
	}
}
