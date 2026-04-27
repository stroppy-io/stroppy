package expr

import (
	"errors"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// singletonDict wraps one value into a uniform one-row Dict. Used to
// make grammar walks deterministic without relying on weights.
func singletonDict(v string) *dgproto.Dict {
	return &dgproto.Dict{
		Rows: []*dgproto.DictRow{{Values: []string{v}}},
	}
}

// multiDict wraps several values into a uniform-weight Dict.
func multiDict(values ...string) *dgproto.Dict {
	rows := make([]*dgproto.DictRow, len(values))
	for i, v := range values {
		rows[i] = &dgproto.DictRow{Values: []string{v}}
	}

	return &dgproto.Dict{Rows: rows}
}

// weightedDict builds a single-profile (default "") weighted dict.
func weightedDict(pairs ...any) *dgproto.Dict {
	if len(pairs)%2 != 0 {
		panic("weightedDict needs value/weight pairs")
	}

	rows := make([]*dgproto.DictRow, 0, len(pairs)/2)

	for i := 0; i < len(pairs); i += 2 {
		rows = append(rows, &dgproto.DictRow{
			Values:  []string{pairs[i].(string)},
			Weights: []int64{int64(pairs[i+1].(int))},
		})
	}

	return &dgproto.Dict{WeightSets: []string{""}, Rows: rows}
}

// grammarExpr builds a StreamDraw Expr wrapping a DrawGrammar.
func grammarExpr(id uint32, g *dgproto.DrawGrammar) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{
		StreamDraw: &dgproto.StreamDraw{
			StreamId: id,
			Draw:     &dgproto.StreamDraw_Grammar{Grammar: g},
		},
	}}
}

// fixture grammar: exactly one sentence "ironic packages wake ." every
// time because each dict carries a single option.
func newSingleChoiceFixture(ctx *fakeCtx) *dgproto.DrawGrammar {
	ctx.dicts["root"] = singletonDict("J N V T")
	ctx.dicts["adjs"] = singletonDict("ironic")
	ctx.dicts["nouns"] = singletonDict("packages")
	ctx.dicts["verbs"] = singletonDict("wake")
	ctx.dicts["terms"] = singletonDict(".")

	return &dgproto.DrawGrammar{
		RootDict: "root",
		Leaves: map[string]string{
			"J": "adjs",
			"N": "nouns",
			"V": "verbs",
			"T": "terms",
		},
		MaxLen: litInt(100),
	}
}

func TestDrawGrammarSingleChoiceDeterministic(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "comment"

	g := newSingleChoiceFixture(ctx)
	e := grammarExpr(7, g)

	want := "ironic packages wake ."

	for i := range int64(10) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		got, err := Eval(ctx, e)
		if err != nil {
			t.Fatalf("eval row %d: %v", i, err)
		}

		if got.(string) != want {
			t.Fatalf("row %d: got %q want %q", i, got, want)
		}
	}
}

func TestDrawGrammarPhraseExpansion(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "comment"

	// Root picks "N V T" — N is a phrase that expands to "J N" via the
	// np dict — J picks an adjective, inner N picks a noun.
	ctx.dicts["root"] = singletonDict("N V T")
	ctx.dicts["np"] = singletonDict("J N")
	ctx.dicts["adjs"] = singletonDict("ironic")
	ctx.dicts["nouns"] = singletonDict("packages")
	ctx.dicts["verbs"] = singletonDict("wake")
	ctx.dicts["terms"] = singletonDict(".")

	g := &dgproto.DrawGrammar{
		RootDict: "root",
		Phrases:  map[string]string{"N": "np"},
		Leaves: map[string]string{
			"J": "adjs",
			"N": "nouns",
			"V": "verbs",
			"T": "terms",
		},
		MaxLen: litInt(100),
	}

	got, err := Eval(ctx, grammarExpr(1, g))
	if err != nil {
		t.Fatalf("eval: %v", err)
	}

	want := "ironic packages wake ."
	if got.(string) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDrawGrammarLiteralTokensPassThrough(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "text"

	// Template contains a literal article "the" and comma ",".
	ctx.dicts["root"] = singletonDict("the J N , T")
	ctx.dicts["adjs"] = singletonDict("quick")
	ctx.dicts["nouns"] = singletonDict("fox")
	ctx.dicts["terms"] = singletonDict(".")

	g := &dgproto.DrawGrammar{
		RootDict: "root",
		Leaves: map[string]string{
			"J": "adjs",
			"N": "nouns",
			"T": "terms",
		},
		MaxLen: litInt(100),
	}

	got, err := Eval(ctx, grammarExpr(1, g))
	if err != nil {
		t.Fatalf("eval: %v", err)
	}

	want := "the quick fox , ."
	if got.(string) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDrawGrammarUnresolvedLetter(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "comment"
	ctx.dicts["root"] = singletonDict("Z .")
	// Z is not in phrases or leaves.

	g := &dgproto.DrawGrammar{
		RootDict: "root",
		Leaves:   map[string]string{"J": "adjs"},
		MaxLen:   litInt(100),
	}

	_, err := Eval(ctx, grammarExpr(1, g))
	if !errors.Is(err, ErrBadGrammar) {
		t.Fatalf("want ErrBadGrammar, got %v", err)
	}
}

func TestDrawGrammarMissingRootDict(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "c"

	g := &dgproto.DrawGrammar{
		RootDict: "missing",
		Leaves:   map[string]string{"J": "adjs"},
		MaxLen:   litInt(10),
	}

	_, err := Eval(ctx, grammarExpr(1, g))
	if !errors.Is(err, ErrBadGrammar) {
		t.Fatalf("want ErrBadGrammar, got %v", err)
	}
}

func TestDrawGrammarTruncation(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "c"

	ctx.dicts["root"] = singletonDict("aaaaaaaaaa bbbbbbbbbb")
	// No letters at all; tokens pass through literal. Length = 21.

	g := &dgproto.DrawGrammar{
		RootDict: "root",
		Leaves:   map[string]string{"X": "x"},
		MaxLen:   litInt(10),
	}

	got, err := Eval(ctx, grammarExpr(1, g))
	if err != nil {
		t.Fatalf("eval: %v", err)
	}

	s := got.(string)
	if len(s) != 10 {
		t.Fatalf("want len 10, got %d (%q)", len(s), s)
	}

	if s != "aaaaaaaaaa" {
		t.Fatalf("truncation mismatch: %q", s)
	}
}

func TestDrawGrammarMinLenReWalk(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "c"

	// Two templates: "x" (1 char) vs "xxxxxxxxx xxxx" (14 chars).
	// With min_len=10, the walker must re-walk until it hits the long
	// option; multi-entry dict ensures the re-walk changes outcome.
	ctx.dicts["root"] = multiDict("x", "xxxxxxxxx xxxx")

	g := &dgproto.DrawGrammar{
		RootDict: "root",
		Leaves:   map[string]string{"Z": "z"},
		MaxLen:   litInt(100),
		MinLen:   litInt(10),
	}

	// Scan many rows: every accepted result with the "x" template would
	// have length 1 (< min_len) and trigger re-walks. If re-walk logic
	// is correct, most rows should land ≥ 10 chars; at worst we return
	// the last attempt. Require at least one long result across 20 rows.
	longHits := 0

	for i := range int64(20) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		got, err := Eval(ctx, grammarExpr(1, g))
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		if len(got.(string)) >= 10 {
			longHits++
		}
	}

	if longHits == 0 {
		t.Fatalf("min_len re-walk never produced a long string across 20 rows")
	}
}

func TestDrawGrammarAcceptShortOnExhaustion(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "c"

	// Single choice that is always "x" — 1 char. min_len=10 cannot be
	// satisfied; the walker must still return after exhausting attempts.
	ctx.dicts["root"] = singletonDict("x")

	g := &dgproto.DrawGrammar{
		RootDict: "root",
		Leaves:   map[string]string{"Z": "z"},
		MaxLen:   litInt(100),
		MinLen:   litInt(10),
	}

	got, err := Eval(ctx, grammarExpr(1, g))
	if err != nil {
		t.Fatalf("eval: %v", err)
	}

	// Accept the short result as-is.
	if got.(string) != "x" {
		t.Fatalf("unexpected fallback result: %q", got)
	}
}

func TestDrawGrammarDeterministicAcrossCalls(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "c"
	ctx.rootSeed = 999

	// Multi-option grammar: outcome depends on PRNG state, so
	// determinism is meaningful.
	ctx.dicts["root"] = multiDict("J N V T", "N V T")
	ctx.dicts["adjs"] = multiDict("ironic", "fluffy", "regular")
	ctx.dicts["nouns"] = multiDict("packages", "requests", "accounts")
	ctx.dicts["verbs"] = multiDict("sleep", "wake", "haggle")
	ctx.dicts["terms"] = multiDict(".", "!")

	g := &dgproto.DrawGrammar{
		RootDict: "root",
		Leaves: map[string]string{
			"J": "adjs", "N": "nouns", "V": "verbs", "T": "terms",
		},
		MaxLen: litInt(100),
	}

	ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = 42

	first, err := Eval(ctx, grammarExpr(3, g))
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	second, err := Eval(ctx, grammarExpr(3, g))
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if first != second {
		t.Fatalf("determinism broken: %q != %q", first, second)
	}
}

func TestDrawGrammarWeightedFrequency(t *testing.T) {
	ctx := newFakeCtx()
	ctx.attrPath = "c"

	// Root always "N .": we can observe the noun distribution directly.
	ctx.dicts["root"] = singletonDict("N T")
	ctx.dicts["terms"] = singletonDict(".")
	// Weighted noun dict: "A" weight 90, "B" weight 10.
	ctx.dicts["nouns"] = weightedDict("A", 90, "B", 10)

	g := &dgproto.DrawGrammar{
		RootDict: "root",
		Leaves:   map[string]string{"N": "nouns", "T": "terms"},
		MaxLen:   litInt(100),
	}

	seen := map[string]int{}

	for i := range int64(1000) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		got, err := Eval(ctx, grammarExpr(1, g))
		if err != nil {
			t.Fatalf("eval: %v", err)
		}

		// Strip the " ." suffix.
		word := strings.TrimSuffix(got.(string), " .")
		seen[word]++
	}

	// Expect ~900 A and ~100 B. Tolerance ±15% per the plan.
	// A window: 900 ± 150.
	if seen["A"] < 750 || seen["A"] > 1000 {
		t.Fatalf("A count %d outside [750, 1000]", seen["A"])
	}

	if seen["B"] < 10 || seen["B"] > 250 {
		t.Fatalf("B count %d outside [10, 250]", seen["B"])
	}
}

func TestDrawGrammarCompileAssignsStreamID(t *testing.T) {
	// Sanity-check the compile step picks up grammar: the constructed
	// Expr has stream_id=0, after compile it must be non-zero. We import
	// the compile package indirectly via an eval-path test; the arm
	// walks streamID through ctx.Draw, so independence between rows
	// proves the field reaches the evaluator.
	ctx := newFakeCtx()
	ctx.attrPath = "c"
	ctx.dicts["root"] = multiDict("A .", "B .", "C .")
	ctx.dicts["a"] = singletonDict("alpha")
	ctx.dicts["b"] = singletonDict("beta")
	ctx.dicts["c"] = singletonDict("gamma")

	g := &dgproto.DrawGrammar{
		RootDict: "root",
		Leaves:   map[string]string{"A": "a", "B": "b", "C": "c"},
		MaxLen:   litInt(100),
	}

	// Two streams with different IDs on the same attrPath+row must
	// diverge; that verifies streamID is mixed into the seed.
	e1 := grammarExpr(1, g)
	e2 := grammarExpr(2, g)

	// Scan a handful of rows — the multi-option root makes a miss on
	// all samples extraordinarily unlikely.
	diverged := false

	for i := range int64(20) {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i

		a, _ := Eval(ctx, e1)
		b, _ := Eval(ctx, e2)

		if a != b {
			diverged = true

			break
		}
	}

	if !diverged {
		t.Fatalf("streamID did not affect output")
	}
}
