package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/next/bench"
	"github.com/stroppy-io/stroppy/next/driver"
	"github.com/stroppy-io/stroppy/next/driver/pg"
	"github.com/stroppy-io/stroppy/next/mem"
	"github.com/stroppy-io/stroppy/next/sqlfile"
)

// rowHash folds one RowBuf row into a 64-bit content hash over all its columns
// (null-aware, type-tagged), used to build order-independent per-table content
// digests for the reproducibility proofs.
func rowHash(b *mem.RowBuf, row int) uint64 {
	h := fnv.New64a()
	var tmp [8]byte
	for col := 0; col < b.Cols(); col++ {
		if b.IsNull(col, row) {
			_, _ = h.Write([]byte{0xff})
			continue
		}
		switch b.Type(col) {
		case mem.TypeInt64:
			binary.LittleEndian.PutUint64(tmp[:], uint64(b.Int64Col(col)[row]))
			_, _ = h.Write(tmp[:])
		case mem.TypeFloat64:
			binary.LittleEndian.PutUint64(tmp[:], math.Float64bits(b.Float64Col(col)[row]))
			_, _ = h.Write(tmp[:])
		case mem.TypeBool:
			var v byte
			if b.BoolCol(col)[row] {
				v = 1
			}
			_, _ = h.Write([]byte{v})
		case mem.TypeBytes:
			_, _ = h.Write(b.BytesAt(col, row))
		}
		_, _ = h.Write([]byte{'|'})
	}
	return h.Sum64()
}

// genTableDigest generates a whole table's rows the way the load handler does —
// walking chunkRanges(nChunks) work items, generating into a reused RowBuf and
// flushing loadBatch-sized batches — and folds every generated row into an
// order-independent digest (a sum of per-row hashes). Because the digest sums
// per-row hashes, it is independent of how the cycle space is partitioned into
// chunks: only the row content, keyed by cycle, feeds it.
func genTableDigest(w *world, tbl *table, nChunks int) uint64 {
	strm := genStreams(tbl)
	buf := mem.NewRowBuf(loadBatch+maxRowsPerCycle, tbl.cols...)
	var acc uint64
	fold := func() {
		for r := 0; r < buf.Rows(); r++ {
			acc += rowHash(buf, r)
		}
		buf.Reset()
	}
	for _, item := range chunkRanges(tbl.cycles(w), nChunks) {
		start, end := parseRange(item)
		for c := start; c < end; c++ {
			tbl.gen(w, buf, c, strm)
			if buf.Rows() >= loadBatch {
				fold()
			}
		}
	}
	if buf.Rows() > 0 {
		fold()
	}
	return acc
}

// TestLoadStreamReproducible is the driverless reproducibility proof (always
// runs, no database): the load's generated row stream is a pure function of
// (seed, cycle), so a fixed seed yields an identical per-table content digest
// across independent runs, and — because content is keyed by the global cycle,
// never by the work partition — that digest is invariant under the chunk count
// (i.e. LOAD_WORKERS), including the LOAD_WORKERS=1 case. This is the structural
// worker-count-invariance guarantee: a generator's signature is
// (world, RowBuf, cycle, streams) — no worker index — so it cannot encode worker
// identity even if it wanted to. This is the fast mirror of
// TestLoadReproduciblePG.
func TestLoadStreamReproducible(t *testing.T) {
	w := newWorld(tpccSeed, 1)
	// Chunk counts span the LOAD_WORKERS space, deliberately including 1 (a
	// single worker) and several uneven values so a partition-dependent bug in
	// the generator cannot hide behind a round worker count.
	chunkCounts := []int{1, 2, 4, 7, 13}
	for _, tbl := range tables() {
		digests := make([]uint64, len(chunkCounts))
		for i, nChunks := range chunkCounts {
			digests[i] = genTableDigest(w, tbl, nChunks)
		}
		for i := 1; i < len(digests); i++ {
			if digests[i] != digests[0] {
				t.Errorf("%s: digest changed with chunk count %d vs %d (%d != %d)",
					tbl.name, chunkCounts[i], chunkCounts[0], digests[i], digests[0])
			}
		}
		if digests[0] == 0 {
			t.Errorf("%s: empty digest (generator produced no rows?)", tbl.name)
		}
	}
}

// TestLoadReproduciblePG is the against-postgres reproducibility proof: it runs
// the real Pool-driven load path twice, with different worker counts, and
// compares an order-independent per-table content aggregate. Identical aggregates
// prove the loaded data is a pure function of (seed, WAREHOUSES) and independent
// of load parallelism. Skipped under -short or without STROPPY_TEST_PG_URL.
func TestLoadReproduciblePG(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped under -short")
	}
	url := os.Getenv("STROPPY_TEST_PG_URL")
	if url == "" {
		t.Skip("set STROPPY_TEST_PG_URL to run the postgres reproducibility test")
	}
	file, err := sqlfile.Parse(tpccSQL)
	if err != nil {
		t.Fatalf("parse tpcc.sql: %v", err)
	}

	a := loadAndDigestPG(t, url, file, 2)
	b := loadAndDigestPG(t, url, file, 5)
	for _, tbl := range tables() {
		if a[tbl.name] != b[tbl.name] {
			t.Errorf("%s: content aggregate differs across LOAD_WORKERS 2 vs 5: %q != %q",
				tbl.name, a[tbl.name], b[tbl.name])
		}
	}
}

// loadAndDigestPG drops and recreates the schema, runs the full per-table Pool
// load with the given worker count, and returns each table's order-independent
// content aggregate (count + sum of per-row hashtext) queried from postgres.
func loadAndDigestPG(t *testing.T, url string, file *sqlfile.File, workers int) map[string]string {
	t.Helper()
	ctx := context.Background()
	w := newWorld(tpccSeed, 1)
	drv := pg.New(driver.Spec{URL: url})
	defer func() { _ = drv.Teardown(ctx) }()

	ddl := func(section string) {
		conn, err := drv.Connect(ctx)
		if err != nil {
			t.Fatalf("connect for %s: %v", section, err)
		}
		defer func() { _ = conn.Close(ctx) }()
		for _, q := range file.Section(section) {
			st, err := conn.Prepare(ctx, q)
			if err != nil {
				t.Fatalf("prepare %s/%s: %v", section, q.Name, err)
			}
			if err := conn.Exec(ctx, st); err != nil {
				t.Fatalf("exec %s/%s: %v", section, q.Name, err)
			}
		}
	}
	ddl("drop_schema")
	ddl("create_schema")

	nChunks := workers * 8
	for _, tbl := range tables() {
		items := chunkRanges(tbl.cycles(w), nChunks)
		cfg := bench.Config{
			Name:     tbl.step(),
			StepID:   loadStepID(tbl.step()),
			Seed:     tpccSeed,
			Drivers:  []driver.Driver{drv},
			Interval: time.Hour,
		}
		ex := bench.Pool(cfg, workers, items, &loadHandler{w: w, tbl: tbl})
		if err := ex.Run(ctx); err != nil {
			t.Fatalf("load %s (workers=%d): %v", tbl.name, workers, err)
		}
	}

	out := make(map[string]string, len(tables()))
	conn, err := drv.Connect(ctx)
	if err != nil {
		t.Fatalf("connect for digest: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, tbl := range tables() {
		// count + sum(hashtext(row)) is order-independent, so it captures content
		// regardless of the physical row order two loads produce.
		q := rawQuery(fmt.Sprintf(
			`SELECT count(*)::text || ':' || coalesce(sum(hashtext(t.*::text)::bigint),0)::text FROM %s t`,
			tbl.name))
		st, err := conn.Prepare(ctx, q)
		if err != nil {
			t.Fatalf("prepare digest %s: %v", tbl.name, err)
		}
		s, err := conn.QueryRow(ctx, st).ScanString(0)
		if err != nil {
			t.Fatalf("digest %s: %v", tbl.name, err)
		}
		out[tbl.name] = s
	}
	return out
}
