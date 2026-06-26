package dsdgen

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// English-language distributions used by generateRandomText. Each is built with
// one value field and one weight field, mirroring EnglishDistributions.java.
var (
	englishAdjectives   = mustLoadEnglish("adjectives.dst")
	englishAdverbs      = mustLoadEnglish("adverbs.dst")
	englishArticles     = mustLoadEnglish("articles.dst")
	englishAuxiliaries  = mustLoadEnglish("auxiliaries.dst")
	englishPrepositions = mustLoadEnglish("prepositions.dst")
	englishNouns        = mustLoadEnglish("nouns.dst")
	englishSentences    = mustLoadEnglish("sentences.dst")
	englishTerminators  = mustLoadEnglish("terminators.dst")
	englishVerbs        = mustLoadEnglish("verbs.dst")
)

// mustLoadEnglish loads a single-value, single-weight distribution applying
// Java's two-stage escaped splitting (colon, then comma), stripping the escaping
// backslashes only once at the end. sentences.dst contains values with escaped
// commas (e.g. "J\, J N VT"), which the generic comma-aware loader cannot keep
// intact because it strips backslashes between the two split passes.
func mustLoadEnglish(file string) *StringValuesDistribution {
	data, err := distFS.ReadFile("distributions/" + file)
	if err != nil {
		panic(err)
	}

	d := &StringValuesDistribution{values: make([][]string, 1), weights: make([][]int, 1)}
	cum := 0

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		parts := splitEscaped(line, ':')
		if len(parts) != 2 {
			panic("dsdgen: " + file + ": expected value:weight, got " + line)
		}
		vals := splitEscaped(parts[0], ',')
		if len(vals) != 1 {
			panic("dsdgen: " + file + ": expected 1 value, got " + line)
		}
		d.values[0] = append(d.values[0], stripBackslashes(strings.TrimSpace(vals[0])))

		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			panic(err)
		}
		cum += n
		d.weights[0] = append(d.weights[0], cum)
	}
	if err := sc.Err(); err != nil {
		panic(err)
	}

	return d
}

// splitEscaped splits s on sep, ignoring separators escaped with a backslash, and
// does NOT strip the backslashes (so nested splits still see the escapes).
func splitEscaped(s string, sep byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep && (i == 0 || s[i-1] != '\\') {
			out = append(out, s[start:i])
			start = i + 1
		}
	}

	return append(out, s[start:])
}

func stripBackslashes(s string) string { return strings.ReplaceAll(s, "\\", "") }

// generateRandomSentence expands one randomly-chosen sentence template into
// words, drawing each part of speech from its distribution. Mirrors
// RandomValueGenerator.generateRandomSentence.
func generateRandomSentence(s *RNStream) string {
	var b strings.Builder
	syntax := englishSentences.PickRandomValue(0, 0, s)
	for i := 0; i < len(syntax); i++ {
		switch syntax[i] {
		case 'N':
			b.WriteString(englishNouns.PickRandomValue(0, 0, s))
		case 'V':
			b.WriteString(englishVerbs.PickRandomValue(0, 0, s))
		case 'J':
			b.WriteString(englishAdjectives.PickRandomValue(0, 0, s))
		case 'D':
			b.WriteString(englishAdverbs.PickRandomValue(0, 0, s))
		case 'X':
			b.WriteString(englishAuxiliaries.PickRandomValue(0, 0, s))
		case 'P':
			b.WriteString(englishPrepositions.PickRandomValue(0, 0, s))
		case 'A':
			b.WriteString(englishArticles.PickRandomValue(0, 0, s))
		case 'T':
			b.WriteString(englishTerminators.PickRandomValue(0, 0, s))
		default:
			b.WriteByte(syntax[i]) // punctuation and whitespace
		}
	}

	return b.String()
}

// GenerateRandomText builds pseudo-random English text whose length falls in
// [minLength, maxLength] by concatenating random sentences and truncating to the
// drawn target length. Mirrors RandomValueGenerator.generateRandomText, including
// its capitalization and trailing-space handling, which are load-bearing for
// byte-exactness.
func GenerateRandomText(minLength, maxLength int, s *RNStream) string {
	isSentenceBeginning := true
	var text strings.Builder
	targetLength := GenerateUniformRandomInt(minLength, maxLength, s)

	for targetLength > 0 {
		generated := generateRandomSentence(s)
		if isSentenceBeginning {
			generated = strings.ToUpper(generated[:1]) + generated[1:]
		}

		generatedLength := len(generated)
		isSentenceBeginning = generated[generatedLength-1] == '.'

		// truncate so as not to exceed target length
		if targetLength < generatedLength {
			generated = generated[:targetLength]
		}

		targetLength -= generatedLength

		text.WriteString(generated)
		if targetLength > 0 {
			text.WriteString(" ")
			targetLength--
		}
	}

	return text.String()
}
