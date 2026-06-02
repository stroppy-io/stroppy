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

// grammarStackBytes covers the common TPC-H text widths without a heap
// buffer; the returned string still owns its bytes via the final copy.
const grammarStackBytes = 256

var grammarAttemptKeys = [grammarMaxAttempts]string{"0", "1", "2", "3", "4", "5", "6", "7"}

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

	var last string

	for attempt := range grammarMaxAttempts {
		walkKey := seed.Derive(rootKey, "grammar", grammarAttemptKeys[attempt])
		prng := seed.PRNG(walkKey)

		out, walkErr := walkGrammar(ctx, prng, grammar, maxLen)
		if walkErr != nil {
			return nil, walkErr
		}

		last = truncateRunes(out, maxLen)
		if runeLenAtLeast(last, minLen) {
			return last, nil
		}
	}

	return last, nil
}

// walkGrammar picks a root template, then walks its tokens: literal
// tokens pass through, single-uppercase-letter tokens resolve through
// phrases (one level) or leaves. Returns ErrBadGrammar when a letter
// resolves through neither map.
func walkGrammar(
	ctx Context,
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	maxLen int64,
) (string, error) {
	return walkGrammarWithResolver(prng, grammar, maxLen, ctx.LookupDict)
}

type grammarDictResolver func(key string) (*dgproto.Dict, error)

func walkGrammarWithResolver(
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	maxLen int64,
	resolve grammarDictResolver,
) (string, error) {
	rootDict, err := resolve(grammar.GetRootDict())
	if err != nil {
		return "", fmt.Errorf("%w: root_dict %q: %w",
			ErrBadGrammar, grammar.GetRootDict(), err)
	}

	rootTemplate, err := pickTemplate(prng, rootDict, grammar.GetRootDict())
	if err != nil {
		return "", err
	}

	out := grammarOutput{
		maxRunes: maxLen,
	}

	if err := appendRootTemplate(&out, prng, grammar, resolve, rootTemplate); err != nil {
		return "", err
	}

	return out.result(), nil
}

type grammarOutput struct {
	stack     [grammarStackBytes]byte
	heap      []byte
	length    int
	maxRunes  int64
	runeCount int64
}

func (out *grammarOutput) writeByte(value byte) {
	if out.runeCount >= out.maxRunes {
		return
	}

	out.appendByte(value)
	out.runeCount++
}

func (out *grammarOutput) writeString(text string) {
	remaining := out.maxRunes - out.runeCount
	if remaining <= 0 || text == "" {
		return
	}

	var runeCount int64

	end := len(text)

	for idx := range text {
		if runeCount == remaining {
			end = idx

			break
		}

		runeCount++
	}

	out.appendBytes(text[:end])
	out.runeCount += runeCount
}

func (out *grammarOutput) appendBytes(text string) {
	if out.heap != nil {
		out.heap = append(out.heap, text...)
		out.length += len(text)

		return
	}

	requiredLen := out.length + len(text)
	if requiredLen <= len(out.stack) {
		copy(out.stack[out.length:], text)
		out.length = requiredLen

		return
	}

	out.heap = make([]byte, out.length, requiredLen)
	copy(out.heap, out.stack[:out.length])
	out.heap = append(out.heap, text...)
	out.length = len(out.heap)
}

func (out *grammarOutput) appendByte(value byte) {
	if out.heap != nil {
		out.heap = append(out.heap, value)
		out.length++

		return
	}

	if out.length < len(out.stack) {
		out.stack[out.length] = value
		out.length++

		return
	}

	out.heap = make([]byte, out.length, out.length+1)
	copy(out.heap, out.stack[:out.length])
	out.heap = append(out.heap, value)
	out.length = len(out.heap)
}

func (out *grammarOutput) result() string {
	if out.heap != nil {
		return string(out.heap[:out.length])
	}

	return string(out.stack[:out.length])
}

func appendRootTemplate(
	out *grammarOutput,
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	resolve grammarDictResolver,
	template string,
) error {
	fieldIndex := 0

	for pos := 0; ; fieldIndex++ {
		tok, nextPos, ok := nextField(template, pos)
		if !ok {
			return nil
		}

		if fieldIndex > 0 {
			out.writeByte(' ')
		}

		letter, ok := grammarLetter(tok)
		if !ok {
			out.writeString(tok)

			pos = nextPos

			continue
		}

		if dictKey, phraseOK := grammar.GetPhrases()[letter]; phraseOK {
			if err := appendPhraseWithResolver(out, prng, grammar, resolve, dictKey, letter); err != nil {
				return err
			}

			pos = nextPos

			continue
		}

		leaf, leafErr := resolveLeafWithResolver(prng, grammar, resolve, letter)
		if leafErr != nil {
			return leafErr
		}

		out.writeString(leaf)

		pos = nextPos
	}
}

func appendPhraseWithResolver(
	out *grammarOutput,
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	resolve grammarDictResolver,
	phraseDictKey string,
	letter string,
) error {
	dict, err := resolve(phraseDictKey)
	if err != nil {
		return fmt.Errorf("%w: phrase dict %q for %q: %w",
			ErrBadGrammar, phraseDictKey, letter, err)
	}

	template, err := pickTemplate(prng, dict, phraseDictKey)
	if err != nil {
		return err
	}

	fieldIndex := 0

	for pos := 0; ; fieldIndex++ {
		tok, nextPos, ok := nextField(template, pos)
		if !ok {
			return nil
		}

		if fieldIndex > 0 {
			out.writeByte(' ')
		}

		subLetter, ok := grammarLetter(tok)
		if !ok {
			out.writeString(tok)

			pos = nextPos

			continue
		}

		leaf, leafErr := resolveLeafWithResolver(prng, grammar, resolve, subLetter)
		if leafErr != nil {
			return leafErr
		}

		out.writeString(leaf)

		pos = nextPos
	}
}

func nextField(text string, pos int) (token string, nextPos int, ok bool) {
	for pos < len(text) {
		r, size := utf8.DecodeRuneInString(text[pos:])
		if !unicode.IsSpace(r) {
			break
		}

		pos += size
	}

	if pos >= len(text) {
		return "", pos, false
	}

	start := pos
	for pos < len(text) {
		r, size := utf8.DecodeRuneInString(text[pos:])
		if unicode.IsSpace(r) {
			break
		}

		pos += size
	}

	return text[start:pos], pos, true
}

func resolveLeafWithResolver(
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	resolve grammarDictResolver,
	letter string,
) (string, error) {
	leafKey, ok := grammar.GetLeaves()[letter]
	if !ok {
		return "", fmt.Errorf("%w: unresolved letter %q", ErrBadGrammar, letter)
	}

	dict, err := resolve(leafKey)
	if err != nil {
		return "", fmt.Errorf("%w: leaf dict %q for %q: %w",
			ErrBadGrammar, leafKey, letter, err)
	}

	return pickTemplate(prng, dict, leafKey)
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

// truncateRunes truncates s to at most n Unicode runes. It counts
// runes rather than bytes because dict contents may carry non-ASCII
// words (e.g. "sauternes", "Tiresias" in the TPC-H grammar).
func truncateRunes(text string, maxRunes int64) string {
	if maxRunes <= 0 {
		return ""
	}

	if int64(len(text)) <= maxRunes {
		return text
	}

	var count int64
	for idx := range text {
		if count == maxRunes {
			return text[:idx]
		}

		count++
	}

	return text
}

func runeLenAtLeast(text string, minRunes int64) bool {
	if minRunes <= 0 {
		return true
	}

	if int64(len(text)) < minRunes {
		return false
	}

	var count int64
	for range text {
		count++
		if count >= minRunes {
			return true
		}
	}

	return false
}
