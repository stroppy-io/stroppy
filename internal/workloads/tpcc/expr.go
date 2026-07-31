package tpcc

import (
	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// Expr builders — the Go struct-literal form of the datagen TS DSL (Rel/Draw/Expr).
// Each returns the proto Expr node the loader evaluates; no DSL port, just the
// trees the TS compiler would have produced.

func litInt(n int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Int64{Int64: n},
	}}}
}

func litFloat(f float64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Double{Double: f},
	}}}
}

func litStr(s string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_String_{String_: s},
	}}}
}

func litNull() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Null{Null: &dgproto.NullMarker{}},
	}}}
}

func col(name string) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Col{Col: &dgproto.ColRef{Name: name}}}
}

// rowIndex is 0-based; rowId is 1-based.
func rowIndex() *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{}}}
}

func rowId() *dgproto.Expr { return add(rowIndex(), litInt(1)) }

func binOp(op dgproto.BinOp_Op, a, b *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_BinOp{BinOp: &dgproto.BinOp{Op: op, A: a, B: b}}}
}

func add(a, b *dgproto.Expr) *dgproto.Expr { return binOp(dgproto.BinOp_ADD, a, b) }
func sub(a, b *dgproto.Expr) *dgproto.Expr { return binOp(dgproto.BinOp_SUB, a, b) }
func mul(a, b *dgproto.Expr) *dgproto.Expr { return binOp(dgproto.BinOp_MUL, a, b) }
func div(a, b *dgproto.Expr) *dgproto.Expr { return binOp(dgproto.BinOp_DIV, a, b) }
func mod(a, b *dgproto.Expr) *dgproto.Expr { return binOp(dgproto.BinOp_MOD, a, b) }

func concat(a, b *dgproto.Expr) *dgproto.Expr { return binOp(dgproto.BinOp_CONCAT, a, b) }
func le(a, b *dgproto.Expr) *dgproto.Expr     { return binOp(dgproto.BinOp_LE, a, b) }
func gt(a, b *dgproto.Expr) *dgproto.Expr     { return binOp(dgproto.BinOp_GT, a, b) }

func ifExpr(cond, then, els *dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_If_{If_: &dgproto.If{Cond: cond, Then: then, Else_: els}}}
}

func call(fn string, args ...*dgproto.Expr) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_Call{Call: &dgproto.Call{Func: fn, Args: args}}}
}

type branch struct {
	weight int64
	expr   *dgproto.Expr
}

// choose builds a weighted pick; streamID keeps the selection draw independent.
func choose(streamID uint32, branches ...branch) *dgproto.Expr {
	bs := make([]*dgproto.ChooseBranch, len(branches))
	for i, b := range branches {
		bs[i] = &dgproto.ChooseBranch{Weight: b.weight, Expr: b.expr}
	}

	return &dgproto.Expr{Kind: &dgproto.Expr_Choose{Choose: &dgproto.Choose{StreamId: streamID, Branches: bs}}}
}

// --- StreamDraw builders ---

func intUniform(min, max int64) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_IntUniform{IntUniform: &dgproto.DrawIntUniform{Min: litInt(min), Max: litInt(max)}},
	}}}
}

func decimal(min, max float64, scale uint32) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Decimal{Decimal: &dgproto.DrawDecimal{Min: litFloat(min), Max: litFloat(max), Scale: scale}},
	}}}
}

func asciiFixed(width int64, alphabet []*dgproto.AsciiRange) *dgproto.Expr {
	return asciiDraw(width, width, alphabet)
}

func asciiRange(minLen, maxLen int64, alphabet []*dgproto.AsciiRange) *dgproto.Expr {
	return asciiDraw(minLen, maxLen, alphabet)
}

func asciiDraw(minLen, maxLen int64, alphabet []*dgproto.AsciiRange) *dgproto.Expr {
	return &dgproto.Expr{Kind: &dgproto.Expr_StreamDraw{StreamDraw: &dgproto.StreamDraw{
		Draw: &dgproto.StreamDraw_Ascii{Ascii: &dgproto.DrawAscii{MinLen: litInt(minLen), MaxLen: litInt(maxLen), Alphabet: alphabet}},
	}}}
}

// Codepoint alphabets (Alphabet.en / enUpper / num from datagen.ts).
var (
	alphaEn      = []*dgproto.AsciiRange{{Min: 65, Max: 90}, {Min: 97, Max: 122}} // [A-Za-z]
	alphaEnUpper = []*dgproto.AsciiRange{{Min: 65, Max: 90}}                      // [A-Z]
	alphaNum     = []*dgproto.AsciiRange{{Min: 48, Max: 57}}                      // [0-9]
)
