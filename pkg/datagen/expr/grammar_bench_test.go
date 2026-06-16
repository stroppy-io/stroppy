package expr

import (
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// benchCommentCtx builds the realistic TPC-H comment grammar used by the e2e
// lineitem benchmark: single-uppercase-letter nonterminals (J adjective, N
// noun, V verb, T terminator) resolved through word dicts whose root templates
// land in the 40..83 char window, so the COMMON walk path is measured rather
// than the re-walk-exhaustion pathology of a letter-free template.
func benchCommentCtx() *fakeCtx {
	ctx := newFakeCtx()
	ctx.attrPath = "l_comment"
	ctx.dicts["g_root"] = multiDict(
		"the J N V the J N T",
		"J N V about the J N T",
		"N V according to the J N T",
		"the J N V quickly the N T",
		"J N V along the J N V T",
	)
	ctx.dicts["g_adj"] = multiDict("furious", "sly", "careful", "blithe", "quick",
		"final", "ironic", "even", "bold", "express", "regular", "special")
	ctx.dicts["g_noun"] = multiDict("packages", "requests", "accounts", "deposits",
		"theodolites", "instructions", "platelets", "foxes", "dolphins",
		"warthogs", "excuses", "dependencies")
	ctx.dicts["g_verb"] = multiDict("wake", "sleep", "cajole", "integrate", "haggle",
		"nag", "sublate", "boost", "detect", "affix", "promise", "snooze")
	ctx.dicts["g_term"] = multiDict(".", "!")

	return ctx
}

func benchCommentGrammar() *dgproto.DrawGrammar {
	return &dgproto.DrawGrammar{
		RootDict: "g_root",
		Leaves:   map[string]string{"J": "g_adj", "N": "g_noun", "V": "g_verb", "T": "g_term"},
		MinLen:   litInt(40),
		MaxLen:   litInt(83),
	}
}

// BenchmarkDrawGrammarComment is the single hottest datagen allocation site
// (~53% of lineitem alloc_objects in the baseline profile): strings.Fields per
// walk, a fresh seed.PRNG (NewPCG) per re-walk attempt, a strings.Builder, and
// []rune truncation. Each iteration generates one comment for a fresh row.
func BenchmarkDrawGrammarComment(b *testing.B) {
	ctx := benchCommentCtx()
	e := grammarExpr(99, benchCommentGrammar())

	b.ReportAllocs()

	var (
		sink string
		i    int64
	)

	for b.Loop() {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = i
		i++

		v, err := Eval(ctx, e)
		if err != nil {
			b.Fatal(err)
		}

		sink, _ = v.(string)
	}

	_ = sink
}
