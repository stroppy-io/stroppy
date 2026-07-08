package main

// Data-generation reproducibility proofs for the lifted dbgen load path. These
// mirror tpcc's TestLoadStreamReproducible: the generated row stream must be a
// pure function of the (table, entity index), so a per-table content digest is
// invariant under the chunk count (== LOAD_WORKERS). They exercise the same
// fresh-seek-per-item model the loadHandler.Iter path drives against a real
// connection, without needing a database.

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"os"
	"testing"

	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/tests/tpch/dbgen"
)

// TestMain initializes the dbgen package once for the whole binary. dbgen's
// distribution tables, tDefs and per-table ranges are lazy globals populated by
// EnsureInit (also the ~300MB text pool); entity-count and BaseRowCount queries
// read those globals, so they must be initialized before any test references
// them. SF=0.01 matches the test scale; EnsureInit is idempotent and tests that
// need another SF re-init freely.
func TestMain(m *testing.M) {
	dbgen.EnsureInit(0.01)
	os.Exit(m.Run())
}

// digestRange generates entity range [start,end) the way loadHandler.Iter does
// (fresh generator seeked to start, walked forward, RowStart/emit/RowStop per
// entity) and folds every emitted row into an order-independent digest: a SUM of
// per-row hashes (each row hashed independently), so the digest is independent
// of how the cycle space is chunked. Because the generator is fresh-seeked per
// range, item [start,end) reproduces byte-identical rows regardless of which
// other ranges ran first — the random-access property that makes the cycle
// space partitionable.
func digestRange(t *dbgenTable, sf float64, start, end int64) (digest uint64, rows int64) {
	g := dbgen.NewGenerator(sf)
	t.seek(g, start)
	buf := mem.NewRowBuf(loadBatch+t.maxRows, t.cols...)
	var tmp [8]byte
	fold := func() {
		for r := 0; r < buf.Rows(); r++ {
			h := fnv.New64a()
			for col := 0; col < buf.Cols(); col++ {
				if buf.IsNull(col, r) {
					h.Write([]byte{0xff})
					continue
				}
				switch buf.Type(col) {
				case mem.TypeInt64:
					binary.LittleEndian.PutUint64(tmp[:], uint64(buf.Int64Col(col)[r]))
					h.Write(tmp[:])
				case mem.TypeFloat64:
					binary.LittleEndian.PutUint64(tmp[:], math.Float64bits(buf.Float64Col(col)[r]))
					h.Write(tmp[:])
				case mem.TypeBool:
					if buf.BoolCol(col)[r] {
						h.Write([]byte{1})
					} else {
						h.Write([]byte{0})
					}
				case mem.TypeBytes:
					h.Write(buf.BytesAt(col, r))
				}
				h.Write([]byte{'|'})
			}
			digest += h.Sum64()
			rows++
		}
		buf.Reset()
	}
	for idx := start; idx < end; idx++ {
		g.RowStart(t.genTable)
		t.emit(g, idx+1, buf)
		g.RowStop(t.genTable)
		if buf.Rows() >= loadBatch {
			fold()
		}
	}
	if buf.Rows() > 0 {
		fold()
	}
	return digest, rows
}

// TestLoadStreamReproducible is the driverless data-repro proof: a fixed table
// at SF=0.01 yields an identical content digest and row count across chunk
// counts {1,3,5}, including the single-chunk (LOAD_WORKERS=1) case. Because
// content is keyed by the global entity index and each chunk fresh-seeks to its
// start, the partition changes only parallelism, never the data — the worker-
// count-invariance guarantee (D11), here proven for the imperative dbgen path.
func TestLoadStreamReproducible(t *testing.T) {
	const sf = 0.01
	chunkCounts := []int{1, 3, 5}
	for _, tbl := range tpchTables() {
		total := tbl.entities(sf)
		var wantDigest uint64
		var wantRows int64
		for i, nChunks := range chunkCounts {
			var digest uint64
			var rows int64
			for _, item := range chunkItems(total, nChunks) {
				d, r := digestRange(tbl, sf, item[0], item[1])
				digest += d
				rows += r
			}
			if i == 0 {
				wantDigest, wantRows = digest, rows
				if rows == 0 {
					t.Fatalf("%s: produced no rows", tbl.name)
				}
				continue
			}
			if digest != wantDigest {
				t.Errorf("%s: content digest changed with chunk count %d vs %d",
					tbl.name, nChunks, chunkCounts[0])
			}
			if rows != wantRows {
				t.Errorf("%s: row count changed with chunk count %d vs %d (%d != %d)",
					tbl.name, nChunks, chunkCounts[0], rows, wantRows)
			}
		}
	}
}

// TestEntityCounts verifies SF scaling against the TPC-H §4.2.2 cardinalities:
// every table scales by the same factor. Dimensions (region/nation) are fixed.
func TestEntityCounts(t *testing.T) {
	cases := []struct {
		name   string
		base   int64
		fixed  bool
	}{
		{"region", dbgen.RegionCount(), true},
		{"nation", dbgen.NationCount(), true},
		{"part", dbgen.BaseRowCount(dbgen.TPart), false},
		{"supplier", dbgen.BaseRowCount(dbgen.TSupp), false},
		{"customer", dbgen.BaseRowCount(dbgen.TCust), false},
		{"orders", dbgen.BaseRowCount(dbgen.TOrder), false},
	}
	for _, c := range cases {
		tbl := findTable(c.name)
		if tbl == nil {
			t.Fatalf("missing table %q", c.name)
		}
		if c.fixed {
			if got := tbl.entities(0.01); got != c.base {
				t.Errorf("%s: entity count %d, want fixed %d", c.name, got, c.base)
			}
			continue
		}
		if got := tbl.entities(1); got != c.base {
			t.Errorf("%s: SF=1 entity count %d, want %d", c.name, got, c.base)
		}
		if got := tbl.entities(0.01); got != c.base/100 {
			t.Errorf("%s: SF=0.01 entity count %d, want %d", c.name, got, c.base/100)
		}
	}
}

func findTable(name string) *dbgenTable {
	for _, t := range tpchTables() {
		if t.name == name {
			return t
		}
	}
	return nil
}

// TestRowCounts verifies the per-table row count at SF=0.01 matches the spec:
// dimensions fixed; partsupp exactly 4×part (suppPerPart); lineitem within a
// band of orders (the degree draw is Uniform(1,7), mean 4, so [2.5,5.5]×orders).
// Computed driverless via digestRange (which returns row counts).
func TestRowCounts(t *testing.T) {
	const sf = 0.01
	rows := make(map[string]int64, len(tpchTables()))
	for _, tbl := range tpchTables() {
		var n int64
		for _, item := range chunkItems(tbl.entities(sf), 3) {
			_, r := digestRange(tbl, sf, item[0], item[1])
			n += r
		}
		rows[tbl.name] = n
	}
	if rows["region"] != dbgen.RegionCount() {
		t.Errorf("region=%d want fixed %d", rows["region"], dbgen.RegionCount())
	}
	if rows["nation"] != dbgen.NationCount() {
		t.Errorf("nation=%d want fixed %d", rows["nation"], dbgen.NationCount())
	}
	if rows["partsupp"] != 4*rows["part"] {
		t.Errorf("partsupp=%d want 4×part=%d", rows["partsupp"], 4*rows["part"])
	}
	lo := int64(2.5 * float64(rows["orders"]))
	hi := int64(5.5 * float64(rows["orders"]))
	if rows["lineitem"] < lo || rows["lineitem"] > hi {
		t.Errorf("lineitem=%d outside [%d,%d] for orders=%d", rows["lineitem"], lo, hi, rows["orders"])
	}
}

// chunkItems splits [0,total) into nChunks contiguous half-open [start,end)
// ranges, mirroring bench.ChunkRanges (kept local to avoid importing bench into
// the digest path, which is deliberately driverless).
func chunkItems(total int64, nChunks int) [][2]int64 {
	if total <= 0 {
		return nil
	}
	if nChunks < 1 {
		nChunks = 1
	}
	if int64(nChunks) > total {
		nChunks = int(total)
	}
	out := make([][2]int64, 0, nChunks)
	size := (total + int64(nChunks) - 1) / int64(nChunks)
	for start := int64(0); start < total; start += size {
		end := start + size
		if end > total {
			end = total
		}
		out = append(out, [2]int64{start, end})
	}
	return out
}
