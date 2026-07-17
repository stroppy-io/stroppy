package mem

import "testing"

func schema() []ColSpec {
	return []ColSpec{
		{Name: "id", Type: TypeInt64},
		{Name: "amount", Type: TypeFloat64},
		{Name: "name", Type: TypeBytes},
		{Name: "active", Type: TypeBool},
	}
}

func TestRowBufAppendAndRead(t *testing.T) {
	b := NewRowBuf(4, schema()...)

	if b.Cols() != 4 {
		t.Fatalf("Cols = %d", b.Cols())
	}
	if b.Name(2) != "name" || b.Type(2) != TypeBytes {
		t.Fatalf("column 2 = %s/%d", b.Name(2), b.Type(2))
	}

	rows := []struct {
		id     int64
		amount float64
		name   string
		active bool
	}{
		{1, 1.5, "alice", true},
		{2, 2.5, "bob", false},
		{3, 3.5, "carol", true},
	}

	for _, r := range rows {
		b.AppendInt64(0, r.id)
		b.AppendFloat64(1, r.amount)
		b.AppendBytes(2, []byte(r.name))
		b.AppendBool(3, r.active)
	}

	if b.Rows() != 3 {
		t.Fatalf("Rows = %d, want 3", b.Rows())
	}

	if got := b.Int64Col(0); len(got) != 3 || got[1] != 2 {
		t.Fatalf("Int64Col = %v", got)
	}
	if got := b.Float64Col(1); got[2] != 3.5 {
		t.Fatalf("Float64Col[2] = %v", got[2])
	}
	if got := b.BoolCol(3); got[0] != true || got[1] != false {
		t.Fatalf("BoolCol = %v", got)
	}
	for i, r := range rows {
		if got := string(b.BytesAt(2, i)); got != r.name {
			t.Fatalf("BytesAt(2,%d) = %q, want %q", i, got, r.name)
		}
	}
}

func TestRowBufNulls(t *testing.T) {
	b := NewRowBuf(8, schema()...)

	b.AppendInt64(0, 10)
	b.AppendNull(1)
	b.AppendBytes(2, []byte("x"))
	b.AppendBool(3, true)

	b.AppendNull(0)
	b.AppendFloat64(1, 9.9)
	b.AppendNull(2)
	b.AppendBool(3, false)

	if !b.IsNull(1, 0) {
		t.Error("row 0 col 1 should be null")
	}
	if b.IsNull(1, 1) {
		t.Error("row 1 col 1 should not be null")
	}
	if !b.IsNull(0, 1) {
		t.Error("row 1 col 0 should be null")
	}
	if !b.IsNull(2, 1) {
		t.Error("row 1 col 2 (bytes) should be null")
	}
	// A null bytes row reads as empty and doesn't disturb neighbours.
	if got := string(b.BytesAt(2, 0)); got != "x" {
		t.Fatalf("BytesAt(2,0) = %q", got)
	}
	if got := b.BytesAt(2, 1); len(got) != 0 {
		t.Fatalf("null bytes row len = %d", len(got))
	}
}

func TestRowBufRowValue(t *testing.T) {
	b := NewRowBuf(4, schema()...)

	b.AppendInt64(0, 7)
	b.AppendFloat64(1, 2.25)
	b.AppendBytes(2, []byte("dan"))
	b.AppendBool(3, false)

	b.AppendNull(0)
	b.AppendNull(1)
	b.AppendNull(2)
	b.AppendNull(3)

	var dst []any
	row0 := b.RowValue(0, dst[:0])
	if len(row0) != 4 {
		t.Fatalf("row0 len = %d, want 4", len(row0))
	}
	if row0[0].(int64) != 7 {
		t.Errorf("row0[0] = %v, want 7", row0[0])
	}
	if row0[1].(float64) != 2.25 {
		t.Errorf("row0[1] = %v, want 2.25", row0[1])
	}
	if string(row0[2].([]byte)) != "dan" {
		t.Errorf("row0[2] = %v, want dan", row0[2])
	}
	if row0[3].(bool) != false {
		t.Errorf("row0[3] = %v, want false", row0[3])
	}

	// dst reused across rows; row1 all-null reads as nil cells.
	row1 := b.RowValue(1, row0[:0])
	if &row1[0] != &row0[0] {
		t.Error("RowValue did not reuse dst backing")
	}
	for col, v := range row1 {
		if v != nil {
			t.Errorf("row1[%d] = %v, want nil", col, v)
		}
	}
}

func TestRowBufResetReuse(t *testing.T) {
	b := NewRowBuf(4, schema()...)

	fill := func() {
		for i := range 4 {
			b.AppendInt64(0, int64(i))
			b.AppendFloat64(1, float64(i))
			b.AppendBytes(2, []byte("val"))
			b.AppendBool(3, i%2 == 0)
		}
	}

	fill()
	col0 := &b.Int64Col(0)[0]

	b.Reset()
	if b.Rows() != 0 {
		t.Fatalf("Rows after Reset = %d", b.Rows())
	}

	fill()
	// Reuses the same backing array.
	if &b.Int64Col(0)[0] != col0 {
		t.Fatal("Reset did not reuse int64 backing")
	}
	// Null bitmap cleared by Reset.
	b.Reset()
	b.AppendInt64(0, 5)
	if b.IsNull(0, 0) {
		t.Fatal("stale null bit after Reset")
	}
}

// All append paths allocate nothing when the batch stays within capacity.
func TestAllocsRowBufAppends(t *testing.T) {
	b := NewRowBuf(256, schema()...)
	name := []byte("customer")

	batch := func() {
		b.Reset()
		for range 256 {
			b.AppendInt64(0, 42)
			b.AppendFloat64(1, 3.14)
			b.AppendBytes(2, name)
			b.AppendBool(3, true)
		}
	}

	batch() // warm to high-water
	batch()

	if n := testing.AllocsPerRun(200, batch); n != 0 {
		t.Errorf("RowBuf append batch allocs = %v, want 0", n)
	}
}

func TestAllocsRowBufNull(t *testing.T) {
	b := NewRowBuf(256, schema()...)

	batch := func() {
		b.Reset()
		for range 256 {
			b.AppendNull(0)
			b.AppendNull(1)
			b.AppendNull(2)
			b.AppendNull(3)
		}
	}
	batch()
	batch()

	if n := testing.AllocsPerRun(200, batch); n != 0 {
		t.Errorf("RowBuf null batch allocs = %v, want 0", n)
	}
}

func BenchmarkRowBufAppendRow(b *testing.B) {
	buf := NewRowBuf(1024, schema()...)
	name := []byte("customer")
	for i := 0; b.Loop(); i++ {
		if buf.Rows() >= 1024 {
			buf.Reset()
		}
		buf.AppendInt64(0, int64(i))
		buf.AppendFloat64(1, 1.0)
		buf.AppendBytes(2, name)
		buf.AppendBool(3, true)
	}
}
