// Package xk6air draw.go — module-scoped registries for the tx-time
// Draw path (iter 2). Dicts, alphabets, and grammars are parsed once
// by RegisterDict / RegisterAlphabet / RegisterGrammar; the NewDrawX
// constructors resolve the resulting pointers eagerly so the hot
// Next()/Sample() calls dereference fields directly. The Drawer
// interface below documents the sobek-bound method set.
package xk6air

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/proto"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// Drawer is the sobek-bound contract for every tx-time Draw arm.
// Returned by the 13 NewDrawX constructors; sobek reflects Sample /
// Next / Seek / Reset onto the JS object as sample / next / seek /
// reset via its FieldNameMapper. The interface is documentary only —
// binding happens by method-set reflection on the concrete pointer.
//
// Concurrency: one struct per VU. k6 gives each VU its own Instance,
// so TS-side construction during init runs once per VU naturally;
// sharing a struct across VUs corrupts the internal cursor.
type Drawer interface {
	// Sample returns the stateless value at (seed, key). It does NOT
	// touch the struct's internal cursor, so Sample and Next can coexist.
	Sample(seed uint64, key int64) any
	// Next returns the value at the current cursor, then advances it.
	Next() any
	// Seek sets the cursor to key (absolute, not relative).
	Seek(key int64)
	// Reset sets the cursor to 0.
	Reset()
}

// drawAttrPath is the fixed seed-path prefix every Draw shares. It
// matches evalContext.Draw's and StatelessContext.Draw's prefix when
// streamID=0 so the three paths bit-match at identical (seed, key).
const (
	drawAttrPath = "draw"
	drawStreamID = "s0"
)

// ErrUnknownDictHandle is returned when a constructor receives a dict
// handle not produced by RegisterDict in this process.
var ErrUnknownDictHandle = errors.New("xk6air: unknown dict handle")

// ErrUnknownAlphabetHandle is returned when a constructor receives an
// alphabet handle not produced by RegisterAlphabet in this process.
var ErrUnknownAlphabetHandle = errors.New("xk6air: unknown alphabet handle")

// ErrUnknownGrammarHandle is returned when a constructor receives a
// grammar handle not produced by RegisterGrammar in this process.
var ErrUnknownGrammarHandle = errors.New("xk6air: unknown grammar handle")

// Module-scoped handle registries. sync.Map wins for our read-heavy
// pattern (register once at init, many hot-path reads).
var (
	dictRegistry sync.Map // uint64 -> *dgproto.Dict
	dictHandleID atomic.Uint64

	namedDicts   sync.Map // string -> *dgproto.Dict (for grammar letter resolution)
	namedDictsMu sync.Mutex

	alphabetRegistry sync.Map // uint64 -> []*dgproto.AsciiRange
	alphabetHandleID atomic.Uint64

	grammarRegistry sync.Map // uint64 -> *dgproto.DrawGrammar
	grammarHandleID atomic.Uint64
)

// RegisterDict stores a serialized Dict in the module registry under
// both a numeric handle (used by NewDrawDict, NewDrawJoint, NewDrawPhrase)
// and a name (used by NewDrawGrammar to resolve letter → dict). Returns
// the numeric handle.
func RegisterDict(name string, dictBin []byte) (uint64, error) {
	d := &dgproto.Dict{}
	if err := proto.Unmarshal(dictBin, d); err != nil {
		return 0, fmt.Errorf("xk6air: unmarshal dict %q: %w", name, err)
	}

	namedDictsMu.Lock()
	namedDicts.Store(name, d)
	namedDictsMu.Unlock()

	id := dictHandleID.Add(1)
	dictRegistry.Store(id, d)

	return id, nil
}

// RegisterAlphabet stores a serialized alphabet (DrawAscii envelope
// carrying only the alphabet field) and returns a handle. NewDrawASCII
// reads the alphabet pointer once at construction.
func RegisterAlphabet(alphabetBin []byte) (uint64, error) {
	var holder dgproto.DrawAscii
	if err := proto.Unmarshal(alphabetBin, &holder); err != nil {
		return 0, fmt.Errorf("xk6air: unmarshal alphabet: %w", err)
	}

	if len(holder.GetAlphabet()) == 0 {
		return 0, fmt.Errorf("xk6air: alphabet empty")
	}

	id := alphabetHandleID.Add(1)
	alphabetRegistry.Store(id, holder.GetAlphabet())

	return id, nil
}

// RegisterGrammar stores a serialized DrawGrammar spec. Its root /
// phrases / leaves dicts must be registered separately via
// RegisterDict (by name) before any grammar NewDrawX constructor runs.
func RegisterGrammar(grammarBin []byte) (uint64, error) {
	g := &dgproto.DrawGrammar{}
	if err := proto.Unmarshal(grammarBin, g); err != nil {
		return 0, fmt.Errorf("xk6air: unmarshal grammar: %w", err)
	}

	id := grammarHandleID.Add(1)
	grammarRegistry.Store(id, g)

	return id, nil
}

// lookupDict returns the dict stored under handle, or an error.
func lookupDict(handle uint64) (*dgproto.Dict, error) {
	raw, ok := dictRegistry.Load(handle)
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownDictHandle, handle)
	}

	d, _ := raw.(*dgproto.Dict)

	return d, nil
}

// lookupAlphabet returns the ranges stored under handle, or an error.
func lookupAlphabet(handle uint64) ([]*dgproto.AsciiRange, error) {
	raw, ok := alphabetRegistry.Load(handle)
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownAlphabetHandle, handle)
	}

	r, _ := raw.([]*dgproto.AsciiRange)

	return r, nil
}

// lookupGrammar returns the grammar stored under handle, or an error.
func lookupGrammar(handle uint64) (*dgproto.DrawGrammar, error) {
	raw, ok := grammarRegistry.Load(handle)
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownGrammarHandle, handle)
	}

	g, _ := raw.(*dgproto.DrawGrammar)

	return g, nil
}

// resolveNamedDict returns the dict registered by name (via
// RegisterDict), or an error when absent. Grammar construction reaches
// this to pre-resolve letter → dict pointers once.
func resolveNamedDict(name string) (*dgproto.Dict, error) {
	raw, ok := namedDicts.Load(name)
	if !ok {
		return nil, fmt.Errorf("xk6air: unknown dict name %q", name)
	}

	d, _ := raw.(*dgproto.Dict)

	return d, nil
}

// toJSDraw converts a Draw kernel's any-typed result into a sobek-
// friendly value. Mirrors toJSValue (defined in generator_wrappers.go)
// but covers the exact return types kernels produce. Kept separate so
// a future refactor of GeneratorWrapper's toJSValue doesn't perturb
// Draw behavior.
func toJSDraw(v any) any {
	switch typed := v.(type) {
	case uuid.UUID:
		return typed.String()
	case *string:
		return *typed
	case time.Time:
		return typed
	case *time.Time:
		return *typed
	case *decimal.Decimal:
		return typed.String()
	default:
		return v
	}
}
