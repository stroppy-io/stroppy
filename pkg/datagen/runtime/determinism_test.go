package runtime

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// TestDeterminismAcrossWorkers enforces CLAUDE.md §Parallelism §1: every
// primitive is a pure function of (rootSeed, attrPath, subKeys, rowIdx)
// and therefore emits an identical row multiset regardless of how the
// row range is sharded across workers. We construct a small spec per
// primitive, drain it via runtime.Clone + SeekRow across workers ∈
// {1, 4, 16}, sort, and require identical multisets. Runs under -race.
//
// The sweep bypasses drivers entirely — the framework owns the seekable
// property; drivers merely fan out chunks.
func TestDeterminismAcrossWorkers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		spec *dgproto.InsertSpec
	}{
		{"literal+binop+dict+if+call+null", mixedFlatSpec(300)},
		{"streamDraw.intUniform", streamDrawFlatSpec(300, &dgproto.StreamDraw_IntUniform{
			IntUniform: &dgproto.DrawIntUniform{Min: litInt64(0), Max: litInt64(1_000_000)},
		})},
		{"streamDraw.floatUniform", streamDrawFlatSpec(300, &dgproto.StreamDraw_FloatUniform{
			FloatUniform: &dgproto.DrawFloatUniform{Min: litFloat64(0), Max: litFloat64(1)},
		})},
		{"streamDraw.normal", streamDrawFlatSpec(300, &dgproto.StreamDraw_Normal{
			Normal: &dgproto.DrawNormal{Min: litFloat64(0), Max: litFloat64(100), Screw: 3},
		})},
		{"streamDraw.zipf", streamDrawFlatSpec(300, &dgproto.StreamDraw_Zipf{
			Zipf: &dgproto.DrawZipf{Min: litInt64(1), Max: litInt64(100), Exponent: 1.3},
		})},
		{"streamDraw.nurand", streamDrawFlatSpec(300, &dgproto.StreamDraw_Nurand{
			Nurand: &dgproto.DrawNURand{A: 255, X: 0, Y: 9999, CSalt: 7},
		})},
		{"streamDraw.bernoulli", streamDrawFlatSpec(300, &dgproto.StreamDraw_Bernoulli{
			Bernoulli: &dgproto.DrawBernoulli{P: 0.3},
		})},
		{"streamDraw.date", streamDrawFlatSpec(300, &dgproto.StreamDraw_Date{
			Date: &dgproto.DrawDate{MinDaysEpoch: 100, MaxDaysEpoch: 400},
		})},
		{"streamDraw.decimal", streamDrawFlatSpec(300, &dgproto.StreamDraw_Decimal{
			Decimal: &dgproto.DrawDecimal{Min: litFloat64(0), Max: litFloat64(100), Scale: 2},
		})},
		{"streamDraw.ascii", streamDrawFlatSpec(300, &dgproto.StreamDraw_Ascii{
			Ascii: &dgproto.DrawAscii{
				MinLen:   litInt64(4),
				MaxLen:   litInt64(8),
				Alphabet: []*dgproto.AsciiRange{{Min: 'a', Max: 'z'}},
			},
		})},
		{"streamDraw.dict", streamDrawDictSpec(300)},
		{"streamDraw.joint", streamDrawJointSpec(300)},
		{"streamDraw.phrase", streamDrawPhraseSpec(300)},
		{"streamDraw.grammar", streamDrawGrammarSpec(300)},
		{"cohort.draw+live", cohortDeterminismSpec(200)},
		{"lookup", lookupDeterminismSpec(200)},
		{"relationship.fixed", relFixedSpec()},
		{"relationship.uniform", relUniformSpec()},
		{"scd2", scd2DeterminismSpec(200)},
	}

	workerCounts := []int{1, 4, 16}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			results := make(map[int][]string, len(workerCounts))

			for _, workers := range workerCounts {
				rows, err := drainParallel(tc.spec, workers)
				if err != nil {
					t.Fatalf("workers=%d: %v", workers, err)
				}

				sort.Strings(rows)
				results[workers] = rows
			}

			baseline := results[1]
			if len(baseline) == 0 {
				t.Fatalf("baseline produced zero rows")
			}

			for _, workers := range workerCounts[1:] {
				if !reflect.DeepEqual(baseline, results[workers]) {
					t.Fatalf("workers=%d produced a different multiset than workers=1",
						workers)
				}
			}
		})
	}
}

// drainParallel builds a Runtime, splits its row range into `workers`
// chunks, drains each chunk via Runtime.Clone + SeekRow in parallel,
// and returns the collected rows rendered as fmt.Sprint strings. Rows
// are not pre-sorted; the caller sorts before comparing multisets.
func drainParallel(spec *dgproto.InsertSpec, workers int) ([]string, error) {
	seed, err := NewRuntime(spec)
	if err != nil {
		return nil, fmt.Errorf("NewRuntime: %w", err)
	}

	totalRows := seed.size

	if workers < 1 {
		workers = 1
	}

	if int64(workers) > totalRows {
		workers = int(totalRows)
	}

	if workers == 0 {
		return nil, nil
	}

	chunk := totalRows / int64(workers)
	remainder := totalRows % int64(workers)

	type bounds struct{ start, count int64 }

	chunks := make([]bounds, workers)

	var cursor int64

	for i := range workers {
		c := chunk
		if int64(i) == int64(workers-1) {
			c += remainder
		}

		chunks[i] = bounds{start: cursor, count: c}
		cursor += c
	}

	var (
		mu   sync.Mutex
		all  []string
		wg   sync.WaitGroup
		errs = make(chan error, workers)
	)

	for i := range workers {
		wg.Add(1)

		go func(b bounds) {
			defer wg.Done()

			w := seed.Clone()
			if err := w.SeekRow(b.start); err != nil {
				errs <- fmt.Errorf("SeekRow(%d): %w", b.start, err)

				return
			}

			local := make([]string, 0, b.count)

			for range b.count {
				row, err := w.Next()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}

					errs <- fmt.Errorf("Next: %w", err)

					return
				}

				local = append(local, fmt.Sprint(row))
			}

			mu.Lock()

			all = append(all, local...)
			mu.Unlock()
		}(chunks[i])
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		if e != nil {
			return nil, e
		}
	}

	return all, nil
}

// mixedFlatSpec exercises the set that the prior common/parallel_insert
// test covered: literal + binop + dict-at + stdlib call + if + nullable.
// Reproduced here so the determinism suite is one file.
func mixedFlatSpec(size int64) *dgproto.InsertSpec {
	dicts := map[string]*dgproto.Dict{
		"regions": {
			Columns: []string{"name"},
			Rows: []*dgproto.DictRow{
				{Values: []string{"africa"}},
				{Values: []string{"america"}},
				{Values: []string{"asia"}},
				{Values: []string{"europe"}},
				{Values: []string{"middle east"}},
			},
		},
	}

	attrs := []*dgproto.Attr{
		attr("row_id", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
		attr("region", dictAt("regions", rowIndex())),
		attr("label", callExpr("std.format", lit("id-%05d"), col("row_id"))),
		attr("bucket", ifExpr(
			binOp(dgproto.BinOp_LT, rowIndex(), lit(int64(100))),
			lit("A"),
			lit("B"),
		)),
		attrWithNull("optional", lit("present"), 0.25, 0xA5A5A5A5DEADBEEF),
	}

	s := spec(size, []string{"row_id", "region", "label", "bucket", "optional"}, attrs, dicts)
	s.Seed = 0xD17A

	return s
}

// streamDrawFlatSpec builds a minimal spec that emits the row index and
// one StreamDraw column. Used for arms that need no ancillary state
// (int/float uniform, normal, zipf, nurand, bernoulli, date, decimal,
// ascii).
func streamDrawFlatSpec(size int64, draw any) *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attr("rowId", rowIndex()),
		attr("v", streamDraw(1, draw)),
	}

	s := spec(size, []string{"rowId", "v"}, attrs, nil)
	s.Seed = 0xD06E

	return s
}

// streamDrawDictSpec wraps DrawDict; requires a dict under "items".
func streamDrawDictSpec(size int64) *dgproto.InsertSpec {
	dicts := map[string]*dgproto.Dict{
		"items": {
			Rows: []*dgproto.DictRow{
				{Values: []string{"alpha"}},
				{Values: []string{"bravo"}},
				{Values: []string{"charlie"}},
				{Values: []string{"delta"}},
			},
		},
	}

	attrs := []*dgproto.Attr{
		attr("rowId", rowIndex()),
		attr("pick", streamDraw(1, &dgproto.StreamDraw_Dict{
			Dict: &dgproto.DrawDict{DictKey: "items"},
		})),
	}

	s := spec(size, []string{"rowId", "pick"}, attrs, dicts)
	s.Seed = 0xD1C7

	return s
}

// streamDrawJointSpec wraps DrawJoint over a two-column dict.
func streamDrawJointSpec(size int64) *dgproto.InsertSpec {
	dicts := map[string]*dgproto.Dict{
		"nations": {
			Columns: []string{"nation", "region_idx"},
			Rows: []*dgproto.DictRow{
				{Values: []string{"ALGERIA", "0"}},
				{Values: []string{"ARGENTINA", "1"}},
				{Values: []string{"BRAZIL", "1"}},
			},
		},
	}

	attrs := []*dgproto.Attr{
		attr("rowId", rowIndex()),
		attr("reg", streamDraw(1, &dgproto.StreamDraw_Joint{
			Joint: &dgproto.DrawJoint{DictKey: "nations", Column: "region_idx"},
		})),
	}

	s := spec(size, []string{"rowId", "reg"}, attrs, dicts)
	s.Seed = 0xD30E

	return s
}

// streamDrawPhraseSpec wraps DrawPhrase over a small vocab.
func streamDrawPhraseSpec(size int64) *dgproto.InsertSpec {
	dicts := map[string]*dgproto.Dict{
		"vocab": {
			Rows: []*dgproto.DictRow{
				{Values: []string{"the"}},
				{Values: []string{"quick"}},
				{Values: []string{"brown"}},
				{Values: []string{"fox"}},
				{Values: []string{"jumps"}},
			},
		},
	}

	attrs := []*dgproto.Attr{
		attr("rowId", rowIndex()),
		attr("text", streamDraw(1, &dgproto.StreamDraw_Phrase{
			Phrase: &dgproto.DrawPhrase{
				VocabKey:  "vocab",
				MinWords:  litInt64(2),
				MaxWords:  litInt64(4),
				Separator: " ",
			},
		})),
	}

	s := spec(size, []string{"rowId", "text"}, attrs, dicts)
	s.Seed = 0xDF8A

	return s
}

// streamDrawGrammarSpec wraps DrawGrammar with a tiny single-expansion
// grammar. One root dict picks the template; leaf dicts fill the tokens.
func streamDrawGrammarSpec(size int64) *dgproto.InsertSpec {
	dicts := map[string]*dgproto.Dict{
		"root":  {Rows: []*dgproto.DictRow{{Values: []string{"J N V T"}}}},
		"adjs":  {Rows: []*dgproto.DictRow{{Values: []string{"ironic"}}, {Values: []string{"final"}}}},
		"nouns": {Rows: []*dgproto.DictRow{{Values: []string{"packages"}}, {Values: []string{"requests"}}}},
		"verbs": {Rows: []*dgproto.DictRow{{Values: []string{"wake"}}, {Values: []string{"sleep"}}}},
		"terms": {Rows: []*dgproto.DictRow{{Values: []string{"."}}}},
	}

	attrs := []*dgproto.Attr{
		attr("rowId", rowIndex()),
		attr("note", streamDraw(1, &dgproto.StreamDraw_Grammar{
			Grammar: &dgproto.DrawGrammar{
				RootDict: "root",
				Leaves: map[string]string{
					"J": "adjs",
					"N": "nouns",
					"V": "verbs",
					"T": "terms",
				},
				MaxLen: litInt64(200),
			},
		})),
	}

	s := spec(size, []string{"rowId", "note"}, attrs, dicts)
	s.Seed = 0xD6AA

	return s
}

// cohortDeterminismSpec exercises CohortDraw + CohortLive on a flat
// spec whose bucket key is the row index. Draws must be pure functions
// of (rootSeed, cohortName, bucket, slot); liveness is a pure function
// of bucket alone.
func cohortDeterminismSpec(size int64) *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attr("rowId", rowIndex()),
		attr("pick", cohortDraw("hot", litInt64(0), rowIndex())),
		attr("live", cohortLive("hot", rowIndex())),
	}

	s := spec(size, []string{"rowId", "pick", "live"}, attrs, nil)
	s.Seed = 0xC087
	s.Source.Cohorts = []*dgproto.Cohort{
		{
			Name:        "hot",
			CohortSize:  16,
			EntityMin:   0,
			EntityMax:   255,
			ActiveEvery: 3,
		},
	}

	return s
}

// lookupDeterminismSpec attaches a LookupPop and reads an attr from it
// on every row. The resolved entity index is the row index modulo the
// pop size.
func lookupDeterminismSpec(size int64) *dgproto.InsertSpec {
	lookupAttrs := []*dgproto.Attr{
		attr("v", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(100)))),
	}

	attrs := []*dgproto.Attr{
		attr("rowId", rowIndex()),
		attr("fk", binOp(dgproto.BinOp_MOD, rowIndex(), lit(int64(50)))),
		attr("looked", &dgproto.Expr{Kind: &dgproto.Expr_Lookup{Lookup: &dgproto.Lookup{
			TargetPop: "ref", AttrName: "v", EntityIndex: col("fk"),
		}}}),
	}

	s := spec(size, []string{"rowId", "fk", "looked"}, attrs, nil)
	s.Seed = 0x10090
	s.Source.LookupPops = []*dgproto.LookupPop{{
		Population:  &dgproto.Population{Name: "ref", Size: 50},
		Attrs:       lookupAttrs,
		ColumnOrder: []string{"v"},
	}}

	return s
}

// relFixedSpec builds a Relationship with Fixed(N) degree on the inner
// side. Row multiset must be insensitive to sharding.
func relFixedSpec() *dgproto.InsertSpec {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 20},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	innerAttrs := []*dgproto.Attr{
		attr("e", rowEntity()),
		attr("i", rowLine()),
	}

	s := relSpec(
		"l", 60,
		innerAttrs,
		[]string{"e", "i"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), fixedSide("l", 3)},
	)
	s.Seed = 0xF1DE

	return s
}

// relUniformSpec builds a Relationship with Uniform(lo, hi) degree.
func relUniformSpec() *dgproto.InsertSpec {
	outer := &dgproto.LookupPop{
		Population:  &dgproto.Population{Name: "o", Size: 30},
		Attrs:       []*dgproto.Attr{attr("k", rowEntity())},
		ColumnOrder: []string{"k"},
	}

	innerAttrs := []*dgproto.Attr{
		attr("e", rowEntity()),
		attr("i", rowLine()),
	}

	s := relSpec(
		"l", 90,
		innerAttrs,
		[]string{"e", "i"},
		outer,
		[]*dgproto.Side{fixedSide("o", 1), uniformSide(1, 5)},
	)
	s.Seed = 0xF1DD

	return s
}

// scd2DeterminismSpec exercises SCD-2 version column injection under
// parallel sharding. The boundary is a constant.
func scd2DeterminismSpec(size int64) *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attr("id", binOp(dgproto.BinOp_ADD, rowIndex(), lit(int64(1)))),
	}

	cfg := &dgproto.SCD2{
		StartCol:        "valid_from",
		EndCol:          "valid_to",
		Boundary:        lit(int64(size / 2)),
		HistoricalStart: lit("1900-01-01"),
		HistoricalEnd:   lit("1999-12-31"),
		CurrentStart:    lit("2000-01-01"),
		CurrentEnd:      lit("9999-12-31"),
	}

	s := scd2Spec(size, attrs, []string{"id", "valid_from", "valid_to"}, cfg)
	s.Seed = 0x5CD2

	return s
}

// --- proto builders local to this determinism suite ------------------------
// (The `expr` package keeps its streamDrawExpr unexported; replicate here.)

// streamDraw wraps any StreamDraw arm into an Expr keyed by `streamID`.
// Accepts an any because the isStreamDraw_Draw interface is unexported.
func streamDraw(streamID uint32, drawArm any) *dgproto.Expr {
	out := &dgproto.StreamDraw{StreamId: streamID}

	switch v := drawArm.(type) {
	case *dgproto.StreamDraw_IntUniform:
		out.Draw = v
	case *dgproto.StreamDraw_FloatUniform:
		out.Draw = v
	case *dgproto.StreamDraw_Normal:
		out.Draw = v
	case *dgproto.StreamDraw_Zipf:
		out.Draw = v
	case *dgproto.StreamDraw_Nurand:
		out.Draw = v
	case *dgproto.StreamDraw_Bernoulli:
		out.Draw = v
	case *dgproto.StreamDraw_Dict:
		out.Draw = v
	case *dgproto.StreamDraw_Joint:
		out.Draw = v
	case *dgproto.StreamDraw_Date:
		out.Draw = v
	case *dgproto.StreamDraw_Decimal:
		out.Draw = v
	case *dgproto.StreamDraw_Ascii:
		out.Draw = v
	case *dgproto.StreamDraw_Phrase:
		out.Draw = v
	case *dgproto.StreamDraw_Grammar:
		out.Draw = v
	default:
		panic(fmt.Sprintf("streamDraw: unknown arm %T", drawArm))
	}

	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: out}}
}

func litInt64(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
}

func litFloat64(f float64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Double{Double: f},
	}}}
}

func cohortDraw(name string, slot, bucketKey *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_CohortDraw{CohortDraw: &dgproto.CohortDraw{
		Name: name, Slot: slot, BucketKey: bucketKey,
	}}}
}

func cohortLive(name string, bucketKey *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_CohortLive{CohortLive: &dgproto.CohortLive{
		Name: name, BucketKey: bucketKey,
	}}}
}
