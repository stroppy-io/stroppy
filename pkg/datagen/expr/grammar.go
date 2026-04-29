package expr

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/seed"
)

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
		prng := seed.PRNG(walkKey)

		out, walkErr := walkGrammar(ctx, prng, grammar)
		if walkErr != nil {
			return nil, walkErr
		}

		last = truncateRunes(out, maxLen)
		if int64(len([]rune(last))) >= minLen {
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
) (string, error) {
	rootDict, err := ctx.LookupDict(grammar.GetRootDict())
	if err != nil {
		return "", fmt.Errorf("%w: root_dict %q: %w",
			ErrBadGrammar, grammar.GetRootDict(), err)
	}

	rootTemplate, err := pickTemplate(prng, rootDict, grammar.GetRootDict())
	if err != nil {
		return "", err
	}

	var out strings.Builder

	for i, tok := range strings.Fields(rootTemplate) {
		if i > 0 {
			out.WriteByte(' ')
		}

		letter, ok := grammarLetter(tok)
		if !ok {
			out.WriteString(tok)

			continue
		}

		if dictKey, phraseOK := grammar.GetPhrases()[letter]; phraseOK {
			expanded, expandErr := expandPhrase(ctx, prng, grammar, dictKey, letter)
			if expandErr != nil {
				return "", expandErr
			}

			out.WriteString(expanded)

			continue
		}

		leaf, leafErr := resolveLeaf(ctx, prng, grammar, letter)
		if leafErr != nil {
			return "", leafErr
		}

		out.WriteString(leaf)
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
	phraseDictKey string,
	letter string,
) (string, error) {
	dict, err := ctx.LookupDict(phraseDictKey)
	if err != nil {
		return "", fmt.Errorf("%w: phrase dict %q for %q: %w",
			ErrBadGrammar, phraseDictKey, letter, err)
	}

	template, err := pickTemplate(prng, dict, phraseDictKey)
	if err != nil {
		return "", err
	}

	var out strings.Builder

	for i, tok := range strings.Fields(template) {
		if i > 0 {
			out.WriteByte(' ')
		}

		subLetter, ok := grammarLetter(tok)
		if !ok {
			out.WriteString(tok)

			continue
		}

		leaf, leafErr := resolveLeaf(ctx, prng, grammar, subLetter)
		if leafErr != nil {
			return "", leafErr
		}

		out.WriteString(leaf)
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

// truncateRunes truncates s to at most n Unicode runes. It counts
// runes rather than bytes because dict contents may carry non-ASCII
// words (e.g. "sauternes", "Tiresias" in the TPC-H grammar).
func truncateRunes(s string, n int64) string {
	if n <= 0 {
		return ""
	}

	runes := []rune(s)
	if int64(len(runes)) <= n {
		return s
	}

	return string(runes[:n])
}
