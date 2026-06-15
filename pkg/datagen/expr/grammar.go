package expr

import (
	"fmt"
	"math/rand/v2"
	"unicode"
	"unicode/utf8"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

// grammarMaxAttempts bounds re-walk attempts when a min_len is set and
// the first walk produces a shorter string. After exhausting attempts,
// drawGrammar returns the last walk result as-is; the spec does not
// require padding.
const grammarMaxAttempts = 8

// keyedDrawer is an optional Context capability: return a PRNG seeded directly
// from a precomputed sub-stream key, reusing the context's pooled PCG instead
// of allocating a fresh source per call. The produced stream is identical to
// seed.PRNG(key) (both route through SeedPCG). evalContext and the lookup
// popCtx implement it; contexts that do not fall back to seed.PRNG.
type keyedDrawer interface {
	DrawKey(key uint64) *rand.Rand
}

// grammarPRNG returns the PRNG for one grammar walk attempt. It reuses the
// context's pooled PCG when available so the re-walk loop allocates no PRNG.
func grammarPRNG(ctx Context, key uint64) *rand.Rand {
	if kd, ok := ctx.(keyedDrawer); ok {
		return kd.DrawKey(key)
	}

	return seed.PRNG(key)
}

// grammarBufferer is an optional Context capability: lend a reusable byte
// buffer for assembling grammar output. Reusing a context-owned buffer across
// rows means a grammar draw allocates only its final, exact-sized result
// string rather than an over-sized builder backing per re-walk attempt.
// evalContext and the lookup popCtx implement it; other contexts get a local
// buffer (one slice per call).
type grammarBufferer interface {
	GrammarScratch() *[]byte
}

// grammarScratch returns a reusable assembly buffer for one grammar draw. For
// contexts that own a buffer it is reused across rows; otherwise a fresh local
// slice is returned.
func grammarScratch(ctx Context) *[]byte {
	if gb, ok := ctx.(grammarBufferer); ok {
		return gb.GrammarScratch()
	}

	var local []byte

	return &local
}

// drawGrammar implements DrawGrammar — a two-phase template walker.
// The walker picks a template from root_dict, splits it on whitespace,
// and for each single-uppercase-ASCII-letter token either:
//
//  1. expands into a phrase template from phrases[letter], whose own
//     letter tokens then resolve through leaves[letter] (one level of
//     phrase recursion only); or
//  2. emits a leaf word from leaves[letter]; or
//  3. returns ErrBadGrammar when the letter resolves into neither.
//
// Literal tokens pass through verbatim. The joined result is truncated
// to `max_len` characters; when `min_len` is set the walker re-walks
// (with a fresh sub-stream per attempt) up to grammarMaxAttempts times
// to satisfy it, and falls back to the final result if none did.
func drawGrammar(
	ctx Context,
	grammar *dgproto.DrawGrammar,
	streamID uint32,
	attrPath string,
	rowIdx int64,
) (any, error) {
	if grammar == nil {
		return nil, ErrBadGrammar
	}

	maxLen, err := evalInt64(ctx, grammar.GetMaxLen())
	if err != nil {
		return nil, err
	}

	if maxLen <= 0 {
		return nil, fmt.Errorf("%w: max_len %d must be > 0", ErrBadGrammar, maxLen)
	}

	minLen := int64(0)

	if grammar.GetMinLen() != nil {
		minLen, err = evalInt64(ctx, grammar.GetMinLen())
		if err != nil {
			return nil, err
		}
	}

	if minLen < 0 {
		return nil, fmt.Errorf("%w: min_len %d must be >= 0", ErrBadGrammar, minLen)
	}

	if minLen > maxLen {
		return nil, fmt.Errorf("%w: min_len %d > max_len %d",
			ErrBadGrammar, minLen, maxLen)
	}

	rootPRNG := ctx.Draw(streamID, attrPath, rowIdx)
	// rootKey gives every re-walk attempt its own sub-stream keyed off
	// the row's single draw. Using the PRNG's own output rather than a
	// reach-around to a private root-seed keeps the evaluator honest:
	// sub-stream derivation flows through seed.Derive, not through a
	// second formula.
	rootKey := rootPRNG.Uint64()

	// buf is a context-owned, reused assembly buffer: after warm-up it holds
	// capacity across rows and attempts, so each walk appends without
	// allocating. Only the truncated result string is freshly allocated (at
	// its exact length) per returned row.
	buf := grammarScratch(ctx)

	var (
		last     string
		lastRune int64
	)

	for attempt := range grammarMaxAttempts {
		walkKey := seed.DeriveGrammarAttempt(rootKey, attempt)
		prng := grammarPRNG(ctx, walkKey)

		*buf = (*buf)[:0]

		if walkErr := walkGrammar(ctx, prng, grammar, buf); walkErr != nil {
			return nil, walkErr
		}

		last, lastRune = truncateRunesToString(*buf, maxLen)
		if lastRune >= minLen {
			return last, nil
		}
	}

	return last, nil
}

// walkGrammar picks a root template, then walks its tokens directly into out:
// literal tokens pass through, single-uppercase-letter tokens resolve through
// phrases (one level) or leaves. Returns ErrBadGrammar when a letter resolves
// through neither map.
func walkGrammar(
	ctx Context,
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	out *[]byte,
) error {
	rootDict, err := ctx.LookupDict(grammar.GetRootDict())
	if err != nil {
		return fmt.Errorf("%w: root_dict %q: %w",
			ErrBadGrammar, grammar.GetRootDict(), err)
	}

	rootTemplate, err := pickTemplate(prng, rootDict, grammar.GetRootDict())
	if err != nil {
		return err
	}

	first := true

	return forEachField(rootTemplate, func(tok string) error {
		if !first {
			*out = append(*out, ' ')
		}

		first = false

		letter, ok := grammarLetter(tok)
		if !ok {
			*out = append(*out, tok...)

			return nil
		}

		if dictKey, phraseOK := grammar.GetPhrases()[letter]; phraseOK {
			return expandPhrase(ctx, prng, grammar, dictKey, letter, out)
		}

		leaf, leafErr := resolveLeaf(ctx, prng, grammar, letter)
		if leafErr != nil {
			return leafErr
		}

		*out = append(*out, leaf...)

		return nil
	})
}

// expandPhrase picks a template from the phrase dict referenced by
// `letter`, splits it into tokens, and resolves every single-letter
// token through the grammar's leaves map, writing directly into out.
// Only one expansion level is permitted: if an expanded token is itself a
// nonterminal, it must resolve into leaves — nested phrase references are
// rejected.
func expandPhrase(
	ctx Context,
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	phraseDictKey string,
	letter string,
	out *[]byte,
) error {
	dict, err := ctx.LookupDict(phraseDictKey)
	if err != nil {
		return fmt.Errorf("%w: phrase dict %q for %q: %w",
			ErrBadGrammar, phraseDictKey, letter, err)
	}

	template, err := pickTemplate(prng, dict, phraseDictKey)
	if err != nil {
		return err
	}

	first := true

	return forEachField(template, func(tok string) error {
		if !first {
			*out = append(*out, ' ')
		}

		first = false

		subLetter, ok := grammarLetter(tok)
		if !ok {
			*out = append(*out, tok...)

			return nil
		}

		leaf, leafErr := resolveLeaf(ctx, prng, grammar, subLetter)
		if leafErr != nil {
			return leafErr
		}

		*out = append(*out, leaf...)

		return nil
	})
}

// resolveLeaf picks a leaf word from the dict referenced by `letter`.
// Returns ErrBadGrammar if the letter has no leaves entry, so walkers
// surface a precise error rather than silently emitting the letter.
func resolveLeaf(
	ctx Context,
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	letter string,
) (string, error) {
	leafKey, ok := grammar.GetLeaves()[letter]
	if !ok {
		return "", fmt.Errorf("%w: unresolved letter %q", ErrBadGrammar, letter)
	}

	dict, err := ctx.LookupDict(leafKey)
	if err != nil {
		return "", fmt.Errorf("%w: leaf dict %q for %q: %w",
			ErrBadGrammar, leafKey, letter, err)
	}

	return pickTemplate(prng, dict, leafKey)
}

// forEachField invokes fn for each whitespace-delimited field of s, matching
// strings.Fields semantics (runs of unicode.IsSpace separate fields; leading,
// trailing, and repeated whitespace are ignored) but without allocating a
// []string — each token is a sub-slice of s. fn returning a non-nil error
// stops iteration and is propagated.
func forEachField(s string, fn func(tok string) error) error {
	start := -1

	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				if err := fn(s[start:i]); err != nil {
					return err
				}

				start = -1
			}

			continue
		}

		if start < 0 {
			start = i
		}
	}

	if start >= 0 {
		return fn(s[start:])
	}

	return nil
}

// pickTemplate draws one row from dict. When the dict declares any
// weight sets, the first one is honored (grammar dicts carry exactly
// one profile — typically named "default" — and the walker's intent
// is "use whatever weights the dict ships"). Dicts with no weight sets
// fall back to uniform.
func pickTemplate(prng *rand.Rand, dict *dgproto.Dict, dictKey string) (string, error) {
	rows := dict.GetRows()
	if len(rows) == 0 {
		return "", fmt.Errorf("%w: empty dict %q", ErrBadGrammar, dictKey)
	}

	profile := ""
	if sets := dict.GetWeightSets(); len(sets) > 0 {
		profile = sets[0]
	}

	idx, err := pickWeightedRow(prng, dict, profile)
	if err != nil {
		return "", fmt.Errorf("%w: dict %q: %w", ErrBadGrammar, dictKey, err)
	}

	values := rows[idx].GetValues()
	if len(values) == 0 {
		return "", fmt.Errorf("%w: dict %q row %d empty",
			ErrBadGrammar, dictKey, idx)
	}

	return values[0], nil
}

// grammarLetter returns (letter, true) when tok is a single uppercase
// ASCII letter (A-Z). The walker only treats such tokens as
// nonterminals; punctuation, commas, articles, and any multi-byte
// token pass through as literals.
func grammarLetter(tok string) (string, bool) {
	if len(tok) != 1 {
		return "", false
	}

	b := tok[0]
	if b < 'A' || b > 'Z' {
		return "", false
	}

	return tok, true
}

// truncateRunes truncates s to at most n Unicode runes without allocating a
// []rune: it scans rune boundaries and slices at the n-th one. It counts runes
// rather than bytes because dict contents may carry non-ASCII words (e.g.
// "sauternes", "Tiresias" in the TPC-H grammar).
func truncateRunes(s string, n int64) string {
	if n <= 0 {
		return ""
	}

	count := int64(0)

	for i := range s {
		if count == n {
			return s[:i]
		}

		count++
	}

	return s
}

// truncateRunesToString returns the first n runes of buf as a freshly
// allocated, exactly-sized string, along with that string's rune count
// (min(n, total runes)). It counts runes rather than bytes so multi-byte dict
// words are never split mid-rune. The returned string copies buf, so the
// caller may reuse buf afterward.
func truncateRunesToString(buf []byte, n int64) (string, int64) {
	if n <= 0 {
		return "", 0
	}

	count := int64(0)

	for i := 0; i < len(buf); {
		if count == n {
			return string(buf[:i]), count
		}

		_, size := utf8.DecodeRune(buf[i:])
		i += size
		count++
	}

	return string(buf), count
}
