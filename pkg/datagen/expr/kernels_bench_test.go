package expr

import (
	"testing"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

var benchStringSink string

func BenchmarkKernelASCII(b *testing.B) {
	alphabet := []*dgproto.AsciiRange{
		{Min: 'A', Max: 'Z'},
		{Min: 'a', Max: 'z'},
		{Min: '0', Max: '9'},
	}
	prng := seed.PRNG(1)

	b.ReportAllocs()

	for range b.N {
		got, err := KernelASCII(prng, 24, 24, alphabet)
		if err != nil {
			b.Fatal(err)
		}

		benchStringSink = got
	}
}

func BenchmarkKernelPhrase(b *testing.B) {
	vocab := benchDict(
		"almond", "antique", "aquamarine", "azure", "beige", "bisque",
		"black", "blush", "brown", "burlywood", "chartreuse", "chiffon",
		"coral", "cornflower", "cyan", "dark", "deep", "dodger",
		"drab", "firebrick", "floral", "forest", "gainsboro", "ghost",
	)
	prng := seed.PRNG(2)

	b.ReportAllocs()

	for range b.N {
		got, err := KernelPhrase(prng, vocab, 8, 8, " ")
		if err != nil {
			b.Fatal(err)
		}

		benchStringSink = got
	}
}

func BenchmarkKernelGrammar(b *testing.B) {
	grammar, dicts := benchGrammar()
	prng := seed.PRNG(3)

	b.ReportAllocs()

	for range b.N {
		got, err := KernelGrammar(prng, grammar, dicts, 0, 96)
		if err != nil {
			b.Fatal(err)
		}

		benchStringSink = got
	}
}

func BenchmarkDrawGrammarEval(b *testing.B) {
	ctx := newFakeCtx()
	ctx.attrPath = "comment"
	grammar, dicts := benchGrammar()

	for key, dict := range dicts {
		ctx.dicts[key] = dict
	}

	expr := grammarExpr(1, grammar)

	b.ReportAllocs()

	for i := range b.N {
		ctx.rowIndex[dgproto.RowIndex_UNSPECIFIED] = int64(i)

		got, err := Eval(ctx, expr)
		if err != nil {
			b.Fatal(err)
		}

		benchStringSink = got.(string)
	}
}

func benchGrammar() (*dgproto.DrawGrammar, map[string]*dgproto.Dict) {
	grammar := &dgproto.DrawGrammar{
		RootDict: "root",
		Phrases: map[string]string{
			"N": "noun_phrases",
		},
		Leaves: map[string]string{
			"J": "adjectives",
			"N": "nouns",
			"P": "prepositions",
			"T": "terminators",
			"V": "verbs",
		},
		MaxLen: benchLitInt(96),
	}

	dicts := map[string]*dgproto.Dict{
		"root": benchDict(
			"J N V P J N T",
			"N V P N T",
			"J N V T",
		),
		"noun_phrases": benchDict(
			"J N",
			"J P N",
			"N",
		),
		"adjectives": benchDict(
			"almond", "antique", "aquamarine", "azure", "beige", "bisque",
			"black", "blush", "brown", "burlywood", "chartreuse", "chiffon",
		),
		"nouns": benchDict(
			"accounts", "asymptotes", "deposits", "packages", "requests", "sleeves",
			"slyly", "theodolites", "waters", "xylophones",
		),
		"prepositions": benchDict("about", "above", "among", "around", "before", "beside"),
		"terminators":  benchDict(".", "!", "?"),
		"verbs":        benchDict("are", "haggle", "sleep", "wake", "x-ray"),
	}

	return grammar, dicts
}

func benchDict(values ...string) *dgproto.Dict {
	rows := make([]*dgproto.DictRow, len(values))
	for i, value := range values {
		rows[i] = &dgproto.DictRow{Values: []string{value}}
	}

	return &dgproto.Dict{Rows: rows}
}

func benchLitInt(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
}
