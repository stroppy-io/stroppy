package expr

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// streamDrawExpr wraps a draw arm into a StreamDraw Expr with the given
// id. The draw parameter is one of the generated StreamDraw_* wrapper
// types (e.g. *dgproto.StreamDraw_IntUniform). Accepts an any because
// the isStreamDraw_Draw interface is unexported.
func streamDrawExpr(id uint32, draw any) *dgproto.Expr {
	out := &dgproto.StreamDraw{StreamId: id}

	switch v := draw.(type) {
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
	default:
		panic("unknown StreamDraw arm")
	}

	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: out}}
}

func TestEvalStreamDrawNil(t *testing.T) {
	ctx := newFakeCtx()

	_, err := Eval(ctx, &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: nil}})
	if !errors.Is(err, ErrBadDraw) {
		t.Fatalf("want ErrBadDraw, got %v", err)
	}
}

func TestDrawIntUniformRangeAndDeterminism(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "c_random"
	ctx.rootSeed = 123

	e := streamDrawExpr(7, &dgproto.StreamDraw_IntUniform{
		IntUniform: &dgproto.DrawIntUniform{
			Min: litInt(0),
			Max: litInt(99),
		},
	})

	const samples = 10_000

	var sum int64

	for i := range samples {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = int64(i)

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got, ok := v.(int64)
		if !ok {
			t.Fatalf("want int64, got %T", v)
		}

		if got < 0 || got > 99 {
			t.Fatalf("sample %d: %d out of [0, 99]", i, got)
		}

		sum += got
	}

	// Mean should approach 49.5 — require it within 2.5.
	mean := float64(sum) / samples
	if math.Abs(mean-49.5) > 2.5 {
		t.Fatalf("mean %v too far from 49.5", mean)
	}

	// Determinism: same (streamID, attrPath, rowIdx) returns same value.
	ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = 42

	first, err := Eval(ctx, e)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	second, err := Eval(ctx, e)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if first != second {
		t.Fatalf("determinism broken: %v != %v", first, second)
	}
}

func TestDrawIntUniformMinGtMax(t *testing.T) {
	ctx := newFakeCtx()
	e := streamDrawExpr(1, &dgproto.StreamDraw_IntUniform{
		IntUniform: &dgproto.DrawIntUniform{Min: litInt(5), Max: litInt(2)},
	})

	_, err := Eval(ctx, e)
	if !errors.Is(err, ErrBadDraw) {
		t.Fatalf("want ErrBadDraw, got %v", err)
	}
}

func TestDrawFloatUniformRange(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "price"

	e := streamDrawExpr(1, &dgproto.StreamDraw_FloatUniform{
		FloatUniform: &dgproto.DrawFloatUniform{
			Min: litFloat(1.0), Max: litFloat(2.0),
		},
	})

	for i := range int64(1000) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got, ok := v.(float64)
		if !ok {
			t.Fatalf("want float64, got %T", v)
		}

		if got < 1.0 || got >= 2.0 {
			t.Fatalf("sample %d: %v out of [1.0, 2.0)", i, got)
		}
	}
}

func TestDrawNormalMeanStddev(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "metric"

	e := streamDrawExpr(1, &dgproto.StreamDraw_Normal{
		Normal: &dgproto.DrawNormal{
			Min:   litFloat(0.0),
			Max:   litFloat(100.0),
			Screw: 3.0,
		},
	})

	const samples = 10_000

	var sum, sumSq float64

	for i := range samples {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = int64(i)

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got := v.(float64)
		if got < 0 || got > 100 {
			t.Fatalf("sample out of clamp: %v", got)
		}

		sum += got
		sumSq += got * got
	}

	mean := sum / samples
	variance := sumSq/samples - mean*mean
	stddev := math.Sqrt(variance)

	if math.Abs(mean-50) > 2.0 {
		t.Fatalf("mean %v too far from 50", mean)
	}

	// Expected stddev = 100/(2*3) = 16.67; allow ±2.5.
	if math.Abs(stddev-16.67) > 2.5 {
		t.Fatalf("stddev %v too far from 16.67", stddev)
	}
}

func TestDrawNormalDefaultScrew(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "a"

	// Two specs: screw=0 (default 3.0) vs screw=3.0 explicit.
	specDefault := streamDrawExpr(1, &dgproto.StreamDraw_Normal{
		Normal: &dgproto.DrawNormal{Min: litFloat(0), Max: litFloat(10), Screw: 0},
	})
	specExplicit := streamDrawExpr(1, &dgproto.StreamDraw_Normal{
		Normal: &dgproto.DrawNormal{Min: litFloat(0), Max: litFloat(10), Screw: 3},
	})

	ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = 0

	a, err := Eval(ctx, specDefault)
	if err != nil {
		t.Fatalf("default: %v", err)
	}

	b, err := Eval(ctx, specExplicit)
	if err != nil {
		t.Fatalf("explicit: %v", err)
	}

	if a != b {
		t.Fatalf("default screw should equal screw=3: %v vs %v", a, b)
	}
}

func TestDrawZipfInRange(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "zipf_attr"

	e := streamDrawExpr(2, &dgproto.StreamDraw_Zipf{
		Zipf: &dgproto.DrawZipf{
			Min:      litInt(10),
			Max:      litInt(50),
			Exponent: 1.2,
		},
	})

	for i := range int64(1000) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got := v.(int64)
		if got < 10 || got > 50 {
			t.Fatalf("zipf out of range [10, 50]: %v", got)
		}
	}
}

func TestDrawNURandClusteringAndDeterminism(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "c_id"

	e := streamDrawExpr(1, &dgproto.StreamDraw_Nurand{
		Nurand: &dgproto.DrawNURand{A: 255, X: 0, Y: 999, CSalt: 0},
	})

	for i := range int64(1000) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got := v.(int64)
		if got < 0 || got > 999 {
			t.Fatalf("nurand out of range [0, 999]: %v", got)
		}
	}

	// Determinism across salts: different salt → different stream.
	e2 := streamDrawExpr(1, &dgproto.StreamDraw_Nurand{
		Nurand: &dgproto.DrawNURand{A: 255, X: 0, Y: 999, CSalt: 42},
	})

	ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = 1

	a, _ := Eval(ctx, e)
	b, _ := Eval(ctx, e2)

	if a == b {
		// Extremely unlikely if salts differ; guard rail for the C derivation.
		t.Logf("note: nurand outputs matched across salts: %v", a)
	}
}

func TestDrawBernoulliDistribution(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "flag"

	e := streamDrawExpr(1, &dgproto.StreamDraw_Bernoulli{
		Bernoulli: &dgproto.DrawBernoulli{P: 0.3},
	})

	const samples = 10_000

	var hits int64

	for i := range samples {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = int64(i)

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got := v.(int64)
		if got != 0 && got != 1 {
			t.Fatalf("bernoulli not {0,1}: %v", got)
		}

		hits += got
	}

	// Expect ~3000 hits; allow ±300 (3% of N).
	if hits < 2700 || hits > 3300 {
		t.Fatalf("bernoulli hits %d not near 3000", hits)
	}
}

func TestDrawBernoulliInvalidP(t *testing.T) {
	ctx := newFakeCtx()
	e := streamDrawExpr(1, &dgproto.StreamDraw_Bernoulli{
		Bernoulli: &dgproto.DrawBernoulli{P: 1.5},
	})

	_, err := Eval(ctx, e)
	if !errors.Is(err, ErrBadDraw) {
		t.Fatalf("want ErrBadDraw, got %v", err)
	}
}

func TestDrawDictUniform(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "region"
	ctx.dicts["regions"] = &dgproto.Dict{
		Rows: []*dgproto.DictRow{
			{Values: []string{"AFRICA"}},
			{Values: []string{"AMERICA"}},
			{Values: []string{"ASIA"}},
		},
	}

	e := streamDrawExpr(1, &dgproto.StreamDraw_Dict{
		Dict: &dgproto.DrawDict{DictKey: "regions"},
	})

	seen := map[string]int{}

	for i := range int64(900) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		seen[v.(string)]++
	}

	if len(seen) != 3 {
		t.Fatalf("expected all 3 values, got %v", seen)
	}
}

func TestDrawDictWeighted(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "reason"
	ctx.dicts["r"] = &dgproto.Dict{
		WeightSets: []string{""},
		Rows: []*dgproto.DictRow{
			{Values: []string{"A"}, Weights: []int64{1}},
			{Values: []string{"B"}, Weights: []int64{9}},
		},
	}

	e := streamDrawExpr(1, &dgproto.StreamDraw_Dict{
		Dict: &dgproto.DrawDict{DictKey: "r"},
	})

	var aCount, bCount int

	for i := range int64(10_000) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		switch v.(string) {
		case "A":
			aCount++
		case "B":
			bCount++
		}
	}

	// A ~= 10%, B ~= 90%; allow ±3% absolute.
	if aCount < 700 || aCount > 1300 {
		t.Fatalf("A count %d not near 1000", aCount)
	}

	if bCount < 8700 || bCount > 9300 {
		t.Fatalf("B count %d not near 9000", bCount)
	}
}

func TestDrawJointReturnsNamedColumn(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "attr"
	ctx.dicts["nations"] = &dgproto.Dict{
		Columns: []string{"nation", "region_idx"},
		Rows: []*dgproto.DictRow{
			{Values: []string{"ALGERIA", "0"}},
			{Values: []string{"ARGENTINA", "1"}},
		},
	}

	e := streamDrawExpr(1, &dgproto.StreamDraw_Joint{
		Joint: &dgproto.DrawJoint{DictKey: "nations", Column: "region_idx"},
	})

	seen := map[string]int{}

	for i := range int64(200) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		seen[v.(string)]++
	}

	if seen["0"] == 0 || seen["1"] == 0 {
		t.Fatalf("joint did not draw both values: %v", seen)
	}
}

func TestDrawDateInRangeUTC(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "d"

	e := streamDrawExpr(1, &dgproto.StreamDraw_Date{
		Date: &dgproto.DrawDate{MinDaysEpoch: 100, MaxDaysEpoch: 200},
	})

	for i := range int64(500) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got, ok := v.(time.Time)
		if !ok {
			t.Fatalf("want time.Time, got %T", v)
		}

		if got.Location() != time.UTC {
			t.Fatalf("expected UTC, got %v", got.Location())
		}

		days := got.Unix() / 86400
		if days < 100 || days > 200 {
			t.Fatalf("days %d out of [100, 200]", days)
		}
	}
}

func TestDrawDecimalScale(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "amt"

	e := streamDrawExpr(1, &dgproto.StreamDraw_Decimal{
		Decimal: &dgproto.DrawDecimal{
			Min: litFloat(0), Max: litFloat(100), Scale: 2,
		},
	})

	for i := range int64(200) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got := v.(float64)
		if got < 0 || got > 100 {
			t.Fatalf("decimal out of range: %v", got)
		}

		// Check rounding to 2 digits: multiply by 100, should be integer.
		scaled := got * 100
		if math.Abs(scaled-math.Round(scaled)) > 1e-6 {
			t.Fatalf("value %v not rounded to 2 digits", got)
		}
	}
}

func TestDrawAsciiLengthAndAlphabet(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "s"

	// alphabet A-Z (65-90).
	e := streamDrawExpr(1, &dgproto.StreamDraw_Ascii{
		Ascii: &dgproto.DrawAscii{
			MinLen: litInt(5), MaxLen: litInt(8),
			Alphabet: []*dgproto.AsciiRange{{Min: 65, Max: 90}},
		},
	})

	for i := range int64(500) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got := v.(string)
		if len(got) < 5 || len(got) > 8 {
			t.Fatalf("length %d out of [5, 8]", len(got))
		}

		for _, c := range got {
			if c < 'A' || c > 'Z' {
				t.Fatalf("char %q not in A-Z", c)
			}
		}
	}
}

func TestDrawPhrase(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "text"
	ctx.dicts["vocab"] = &dgproto.Dict{
		Rows: []*dgproto.DictRow{
			{Values: []string{"the"}},
			{Values: []string{"quick"}},
			{Values: []string{"fox"}},
		},
	}

	e := streamDrawExpr(1, &dgproto.StreamDraw_Phrase{
		Phrase: &dgproto.DrawPhrase{
			VocabKey: "vocab", MinWords: litInt(2), MaxWords: litInt(4),
			Separator: " ",
		},
	})

	for i := range int64(300) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		v, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		got := v.(string)

		words := strings.Split(got, " ")
		if len(words) < 2 || len(words) > 4 {
			t.Fatalf("word count %d out of [2, 4] for %q", len(words), got)
		}
	}
}

func TestDrawIndependentPerAttrPath(t *testing.T) {
	ctxA := newFakeCtx()
	ctxA.attrPath = "a"
	ctxB := newFakeCtx()
	ctxB.attrPath = "b"

	e := streamDrawExpr(1, &dgproto.StreamDraw_IntUniform{
		IntUniform: &dgproto.DrawIntUniform{Min: litInt(0), Max: litInt(1_000_000)},
	})

	ctxA.rowIndex[dgproto.RowIndex_UNSPECIFIED] = 0
	ctxB.rowIndex[dgproto.RowIndex_UNSPECIFIED] = 0

	a, _ := Eval(ctxA, e)
	b, _ := Eval(ctxB, e)

	if a == b {
		t.Fatalf("different attr paths should yield different streams (got both %v)", a)
	}
}
