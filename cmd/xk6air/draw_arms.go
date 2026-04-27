// Package xk6air draw_arms.go — 12 concrete Drawer structs, one per
// StreamDraw oneof arm. Each struct stores its pre-resolved literal
// bounds (and, for dict-bearing arms, pre-resolved pointers) so Next
// and Sample dereference fields directly and call the matching
// kernels.*. No expr.Eval on the hot path; no per-call alloc beyond
// what the kernel itself does.
package xk6air

import (
	"strconv"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/expr"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// drawKey folds (rootSeed, key) into the PRNG seed, matching the
// composition the full evaluator performs via ctx.Draw(streamID=0,
// attrPath="draw", rowIdx=key). Inlined so every Next()/Sample() hot
// path hits a single Derive call.
func drawKey(rootSeed uint64, key int64) uint64 {
	return seed.Derive(rootSeed, drawAttrPath, drawStreamID, strconv.FormatInt(key, 10))
}

// drawIntUniform is the sobek-bound tx-time generator for IntUniform.
// Field layout is identical across arms: {seed, cursor, ...arm-specific}.
type drawIntUniform struct {
	seed   uint64
	cursor int64
	lo, hi int64
}

func (d *drawIntUniform) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelIntUniform(p.r, d.lo, d.hi)
	releasePRNG(p)
	return v
}

func (d *drawIntUniform) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawIntUniform) Seek(key int64) { d.cursor = key }
func (d *drawIntUniform) Reset()         { d.cursor = 0 }

// drawFloatUniform — FloatUniform arm.
type drawFloatUniform struct {
	seed   uint64
	cursor int64
	lo, hi float64
}

func (d *drawFloatUniform) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelFloatUniform(p.r, d.lo, d.hi)
	releasePRNG(p)
	return v
}

func (d *drawFloatUniform) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawFloatUniform) Seek(key int64) { d.cursor = key }
func (d *drawFloatUniform) Reset()         { d.cursor = 0 }

// drawNormal — Normal arm.
type drawNormal struct {
	seed   uint64
	cursor int64
	lo, hi float64
	screw  float32
}

func (d *drawNormal) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelNormal(p.r, d.lo, d.hi, d.screw)
	releasePRNG(p)
	return v
}

func (d *drawNormal) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawNormal) Seek(key int64) { d.cursor = key }
func (d *drawNormal) Reset()         { d.cursor = 0 }

// drawZipf — Zipf arm.
type drawZipf struct {
	seed     uint64
	cursor   int64
	lo, hi   int64
	exponent float64
}

func (d *drawZipf) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelZipf(p.r, d.lo, d.hi, d.exponent)
	releasePRNG(p)
	return v
}

func (d *drawZipf) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawZipf) Seek(key int64) { d.cursor = key }
func (d *drawZipf) Reset()         { d.cursor = 0 }

// drawNURand — Nurand arm.
type drawNURand struct {
	seed    uint64
	cursor  int64
	a, x, y int64
	cSalt   uint64
}

func (d *drawNURand) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelNURand(p.r, d.a, d.x, d.y, d.cSalt)
	releasePRNG(p)
	return v
}

func (d *drawNURand) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawNURand) Seek(key int64) { d.cursor = key }
func (d *drawNURand) Reset()         { d.cursor = 0 }

// drawBernoulli — Bernoulli arm.
type drawBernoulli struct {
	seed   uint64
	cursor int64
	p      float32
}

func (d *drawBernoulli) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelBernoulli(p.r, d.p)
	releasePRNG(p)
	return v
}

func (d *drawBernoulli) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawBernoulli) Seek(key int64) { d.cursor = key }
func (d *drawBernoulli) Reset()         { d.cursor = 0 }

// drawDate — Date arm. Bounds are already days-since-epoch.
type drawDate struct {
	seed           uint64
	cursor         int64
	loDays, hiDays int64
}

func (d *drawDate) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelDate(p.r, d.loDays, d.hiDays)
	releasePRNG(p)
	return toJSDraw(v)
}

func (d *drawDate) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawDate) Seek(key int64) { d.cursor = key }
func (d *drawDate) Reset()         { d.cursor = 0 }

// drawDecimal — Decimal arm.
type drawDecimal struct {
	seed   uint64
	cursor int64
	lo, hi float64
	scale  uint32
}

func (d *drawDecimal) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelDecimal(p.r, d.lo, d.hi, d.scale)
	releasePRNG(p)
	return v
}

func (d *drawDecimal) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawDecimal) Seek(key int64) { d.cursor = key }
func (d *drawDecimal) Reset()         { d.cursor = 0 }

// drawASCII — Ascii arm. Alphabet resolved once at construction.
type drawASCII struct {
	seed            uint64
	cursor          int64
	minLen, maxLen  int64
	alphabet        []*dgproto.AsciiRange
}

func (d *drawASCII) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelASCII(p.r, d.minLen, d.maxLen, d.alphabet)
	releasePRNG(p)
	return v
}

func (d *drawASCII) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawASCII) Seek(key int64) { d.cursor = key }
func (d *drawASCII) Reset()         { d.cursor = 0 }

// drawDict — Dict arm. Dict pointer resolved once at construction.
type drawDict struct {
	seed      uint64
	cursor    int64
	dict      *dgproto.Dict
	weightSet string
}

func (d *drawDict) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelDict(p.r, d.dict, d.weightSet)
	releasePRNG(p)
	return toJSDraw(v)
}

func (d *drawDict) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawDict) Seek(key int64) { d.cursor = key }
func (d *drawDict) Reset()         { d.cursor = 0 }

// drawJoint — Joint arm. Dict pointer + column index pre-resolved.
type drawJoint struct {
	seed      uint64
	cursor    int64
	dict      *dgproto.Dict
	colIdx    int
	weightSet string
}

func (d *drawJoint) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelJoint(p.r, d.dict, d.colIdx, d.weightSet)
	releasePRNG(p)
	return toJSDraw(v)
}

func (d *drawJoint) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawJoint) Seek(key int64) { d.cursor = key }
func (d *drawJoint) Reset()         { d.cursor = 0 }

// drawPhrase — Phrase arm. Vocab pointer resolved once at construction.
type drawPhrase struct {
	seed         uint64
	cursor       int64
	vocab        *dgproto.Dict
	minW, maxW   int64
	sep          string
}

func (d *drawPhrase) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelPhrase(p.r, d.vocab, d.minW, d.maxW, d.sep)
	releasePRNG(p)
	return v
}

func (d *drawPhrase) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawPhrase) Seek(key int64) { d.cursor = key }
func (d *drawPhrase) Reset()         { d.cursor = 0 }

// drawGrammar — Grammar arm. All letter → dict pointers pre-resolved at
// construction; the kernel walks the grammar using only the map.
type drawGrammar struct {
	seed           uint64
	cursor         int64
	grammar        *dgproto.DrawGrammar
	dicts          map[string]*dgproto.Dict
	minLen, maxLen int64
}

func (d *drawGrammar) Sample(rootSeed uint64, key int64) any {
	p := acquirePRNG(drawKey(rootSeed, key))
	v, _ := expr.KernelGrammar(p.r, d.grammar, d.dicts, d.minLen, d.maxLen)
	releasePRNG(p)
	return v
}

func (d *drawGrammar) Next() any {
	v := d.Sample(d.seed, d.cursor)
	d.cursor++
	return v
}

func (d *drawGrammar) Seek(key int64) { d.cursor = key }
func (d *drawGrammar) Reset()         { d.cursor = 0 }

// Compile-time guards: every struct must satisfy the Drawer contract.
var (
	_ Drawer = (*drawIntUniform)(nil)
	_ Drawer = (*drawFloatUniform)(nil)
	_ Drawer = (*drawNormal)(nil)
	_ Drawer = (*drawZipf)(nil)
	_ Drawer = (*drawNURand)(nil)
	_ Drawer = (*drawBernoulli)(nil)
	_ Drawer = (*drawDate)(nil)
	_ Drawer = (*drawDecimal)(nil)
	_ Drawer = (*drawASCII)(nil)
	_ Drawer = (*drawDict)(nil)
	_ Drawer = (*drawJoint)(nil)
	_ Drawer = (*drawPhrase)(nil)
	_ Drawer = (*drawGrammar)(nil)
)
