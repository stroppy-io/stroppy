package expr

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

type keyedDrawer interface {
	DrawKey(key uint64) *rand.Rand
}

// grammarMaxAttempts bounds re-walk attempts when a min_len is set and
// the first walk produces a shorter string. After exhausting attempts,
// drawGrammar returns the last walk result as-is; the spec does not
// require padding.
const grammarMaxAttempts = 8

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

	rootDict, err := ctx.LookupDict(grammar.GetRootDict())
	if err != nil {
		return "", fmt.Errorf("%w: root_dict %q: %w", ErrBadGrammar, grammar.GetRootDict(), err)
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
		walkKey := seed.Derive(rootKey, "grammar", strconv.Itoa(attempt))
		prng := prngForKey(ctx, walkKey)

		out, walkErr := walkGrammar(ctx, prng, grammar, rootDict)
		if walkErr != nil {
			return nil, walkErr
		}

		last = truncateRunes(out, maxLen)
		if runeCount(last) >= minLen {
			return last, nil
		}
	}

	return last, nil
}

func prngForKey(ctx Context, key uint64) *rand.Rand {
	if drawer, ok := ctx.(keyedDrawer); ok {
		return drawer.DrawKey(key)
	}

	return seed.PRNG(key)
}

// walkGrammar walks a template using pre-computed tokens from dict.TokenizedTemplates.
func walkGrammar(
	ctx Context,
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	rootDict *dgproto.Dict,
) (string, error) {
	templateIdx, err := pickWeightedRow(prng, rootDict, "")
	if err != nil {
		return "", fmt.Errorf("%w: root template pick: %w", ErrBadGrammar, err)
	}

	template := rootDict.GetRows()[templateIdx].GetValues()[0]
	if len(rootDict.TokenizedTemplates) > templateIdx {
		template = rootDict.TokenizedTemplates[templateIdx]
	}

	if !hasGrammarToken(template) {
		return template, nil
	}

	var out strings.Builder

	if err := forEachToken(template, func(i int, tok string) error {
		if i > 0 {
			out.WriteByte(' ')
		}

		letter, ok := grammarLetter(tok)
		if !ok {
			out.WriteString(tok)

			return nil
		}

		if dictKey, phraseOK := grammar.GetPhrases()[letter]; phraseOK {
			phraseDict, err := ctx.LookupDict(dictKey)
			if err != nil {
				return fmt.Errorf("%w: phrase dict %q for %q: %w", ErrBadGrammar, dictKey, letter, err)
			}

			expanded, expandErr := expandPhrase(ctx, prng, grammar, phraseDict)
			if expandErr != nil {
				return expandErr
			}

			out.WriteString(expanded)

			return nil
		}

		leaf, leafErr := resolveLeaf(ctx, prng, grammar, letter)
		if leafErr != nil {
			return leafErr
		}

		out.WriteString(leaf)

		return nil
	}); err != nil {
		return "", err
	}

	return out.String(), nil
}

// expandPhrase picks a template from the phrase dict referenced by
// `letter`, splits it into tokens, and resolves every single-letter
// token through the grammar's leaves map. Only one expansion level is
// permitted: if an expanded token is itself a nonterminal, it must
// resolve into leaves — nested phrase references are rejected.
func expandPhrase(
	ctx Context,
	prng *rand.Rand,
	grammar *dgproto.DrawGrammar,
	dict *dgproto.Dict,
) (string, error) {
	templateIdx, err := pickWeightedRow(prng, dict, "")
	if err != nil {
		return "", fmt.Errorf("%w: template pick: %w", ErrBadGrammar, err)
	}

	template := dict.GetRows()[templateIdx].GetValues()[0]
	if len(dict.TokenizedTemplates) > templateIdx {
		template = dict.TokenizedTemplates[templateIdx]
	}

	if !hasGrammarToken(template) {
		return template, nil
	}

	var out strings.Builder

	if err := forEachToken(template, func(i int, tok string) error {
		if i > 0 {
			out.WriteByte(' ')
		}

		subLetter, ok := grammarLetter(tok)
		if !ok {
			out.WriteString(tok)

			return nil
		}

		leaf, leafErr := resolveLeaf(ctx, prng, grammar, subLetter)
		if leafErr != nil {
			return leafErr
		}

		out.WriteString(leaf)

		return nil
	}); err != nil {
		return "", err
	}

	return out.String(), nil
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

func forEachToken(template string, fn func(i int, tok string) error) error {
	start := -1
	idx := 0

	for i := range len(template) {
		if template[i] == ' ' || template[i] == '\t' || template[i] == '\n' || template[i] == '\r' {
			if start >= 0 {
				if err := fn(idx, template[start:i]); err != nil {
					return err
				}

				idx++
				start = -1
			}

			continue
		}

		if start < 0 {
			start = i
		}
	}

	if start >= 0 {
		return fn(idx, template[start:])
	}

	return nil
}

func hasGrammarToken(template string) bool {
	found := false
	_ = forEachToken(template, func(_ int, tok string) error {
		_, ok := grammarLetter(tok)
		found = found || ok

		return nil
	})

	return found
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

func runeCount(s string) int64 {
	count := utf8.RuneCountInString(s)
	if count == len(s) {
		return int64(len(s))
	}

	return int64(count)
}
