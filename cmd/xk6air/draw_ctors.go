// Package xk6air draw_ctors.go — 13 exported constructor functions
// (NewDrawX). Each resolves handles and validates bounds once, then
// returns a *drawX pointer that sobek binds by reflection. Errors
// return as any so sobek converts them to a JS exception.
package xk6air

import (
	"fmt"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
)

// NewDrawIntUniform constructs an IntUniform-arm sobek handle.
func NewDrawIntUniform(seed uint64, lo, hi int64) any {
	if lo > hi {
		return fmt.Errorf("xk6air: int_uniform lo %d > hi %d", lo, hi)
	}
	return &drawIntUniform{seed: seed, lo: lo, hi: hi}
}

// NewDrawFloatUniform constructs a FloatUniform-arm sobek handle.
func NewDrawFloatUniform(seed uint64, lo, hi float64) any {
	if lo >= hi {
		return fmt.Errorf("xk6air: float_uniform lo %v >= hi %v", lo, hi)
	}
	return &drawFloatUniform{seed: seed, lo: lo, hi: hi}
}

// NewDrawNormal constructs a Normal-arm sobek handle.
func NewDrawNormal(seed uint64, lo, hi float64, screw float32) any {
	if lo >= hi {
		return fmt.Errorf("xk6air: normal lo %v >= hi %v", lo, hi)
	}
	return &drawNormal{seed: seed, lo: lo, hi: hi, screw: screw}
}

// NewDrawZipf constructs a Zipf-arm sobek handle.
func NewDrawZipf(seed uint64, lo, hi int64, exponent float64) any {
	if lo > hi {
		return fmt.Errorf("xk6air: zipf lo %d > hi %d", lo, hi)
	}
	return &drawZipf{seed: seed, lo: lo, hi: hi, exponent: exponent}
}

// NewDrawNURand constructs a Nurand-arm sobek handle. cSalt=0 yields
// the deterministic default C used by TPC-C main.
func NewDrawNURand(seed uint64, a, x, y int64, cSalt uint64) any {
	if a < 0 || x < 0 || y < x {
		return fmt.Errorf("xk6air: nurand A=%d x=%d y=%d", a, x, y)
	}
	return &drawNURand{seed: seed, a: a, x: x, y: y, cSalt: cSalt}
}

// NewDrawBernoulli constructs a Bernoulli-arm sobek handle.
func NewDrawBernoulli(seed uint64, p float32) any {
	if p < 0 || p > 1 {
		return fmt.Errorf("xk6air: bernoulli p=%v out of [0,1]", p)
	}
	return &drawBernoulli{seed: seed, p: p}
}

// NewDrawDate constructs a Date-arm sobek handle. Bounds are already
// days-since-epoch (TS-side conversion via std.dateToDays).
func NewDrawDate(seed uint64, loDays, hiDays int64) any {
	if loDays > hiDays {
		return fmt.Errorf("xk6air: date lo %d > hi %d", loDays, hiDays)
	}
	return &drawDate{seed: seed, loDays: loDays, hiDays: hiDays}
}

// NewDrawDecimal constructs a Decimal-arm sobek handle.
func NewDrawDecimal(seed uint64, lo, hi float64, scale uint32) any {
	if lo > hi {
		return fmt.Errorf("xk6air: decimal lo %v > hi %v", lo, hi)
	}
	return &drawDecimal{seed: seed, lo: lo, hi: hi, scale: scale}
}

// NewDrawASCII constructs an Ascii-arm sobek handle. The alphabet is
// pre-registered via RegisterAlphabet.
func NewDrawASCII(seed uint64, minLen, maxLen int64, alphabetHandle uint64) any {
	if minLen < 0 || maxLen < minLen {
		return fmt.Errorf("xk6air: ascii lens [%d, %d] invalid", minLen, maxLen)
	}

	alpha, err := lookupAlphabet(alphabetHandle)
	if err != nil {
		return err
	}

	return &drawASCII{seed: seed, minLen: minLen, maxLen: maxLen, alphabet: alpha}
}

// NewDrawDict constructs a Dict-arm sobek handle.
func NewDrawDict(seed uint64, dictHandle uint64, weightSet string) any {
	dict, err := lookupDict(dictHandle)
	if err != nil {
		return err
	}

	return &drawDict{seed: seed, dict: dict, weightSet: weightSet}
}

// NewDrawJoint constructs a Joint-arm sobek handle. Column index is
// pre-resolved; unknown columns error at construction.
func NewDrawJoint(seed uint64, dictHandle uint64, column, weightSet string) any {
	dict, err := lookupDict(dictHandle)
	if err != nil {
		return err
	}

	colIdx := expr.LookupJointColumn(dict, column)
	if colIdx < 0 {
		return fmt.Errorf("xk6air: joint dict missing column %q", column)
	}

	return &drawJoint{seed: seed, dict: dict, colIdx: colIdx, weightSet: weightSet}
}

// NewDrawPhrase constructs a Phrase-arm sobek handle.
func NewDrawPhrase(seed uint64, vocabHandle uint64, minW, maxW int64, sep string) any {
	if minW < 1 || maxW < minW {
		return fmt.Errorf("xk6air: phrase words [%d, %d] invalid", minW, maxW)
	}

	vocab, err := lookupDict(vocabHandle)
	if err != nil {
		return err
	}

	return &drawPhrase{seed: seed, vocab: vocab, minW: minW, maxW: maxW, sep: sep}
}

// NewDrawGrammar constructs a Grammar-arm sobek handle. All letter →
// dict pointers are pre-resolved against the named-dict registry so
// the hot path never touches sync.Map.
func NewDrawGrammar(seed uint64, grammarHandle uint64, minLen, maxLen int64) any {
	if maxLen <= 0 {
		return fmt.Errorf("xk6air: grammar max_len %d must be > 0", maxLen)
	}

	if minLen < 0 || minLen > maxLen {
		return fmt.Errorf("xk6air: grammar lens [%d, %d] invalid", minLen, maxLen)
	}

	g, err := lookupGrammar(grammarHandle)
	if err != nil {
		return err
	}

	dicts, err := resolveGrammarDicts(g)
	if err != nil {
		return err
	}

	return &drawGrammar{
		seed:    seed,
		grammar: g,
		dicts:   dicts,
		minLen:  minLen,
		maxLen:  maxLen,
	}
}

// resolveGrammarDicts builds a letter → *Dict map for the grammar's
// root + phrases + leaves entries, resolving each against the named-
// dict registry. Errors cite the missing dict name so TS catch blocks
// can surface a precise cause.
func resolveGrammarDicts(g *dgproto.DrawGrammar) (map[string]*dgproto.Dict, error) {
	names := map[string]struct{}{g.GetRootDict(): {}}

	for _, v := range g.GetPhrases() {
		names[v] = struct{}{}
	}

	for _, v := range g.GetLeaves() {
		names[v] = struct{}{}
	}

	out := make(map[string]*dgproto.Dict, len(names))

	for name := range names {
		d, err := resolveNamedDict(name)
		if err != nil {
			return nil, err
		}

		out[name] = d
	}

	return out, nil
}
