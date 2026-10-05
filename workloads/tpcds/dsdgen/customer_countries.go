package dsdgen

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// mustLoadCountries loads countries.dst (1 value field, 1 cumulative weight
// field) like mustLoadEnglish, but additionally transcodes the country names
// from ISO-8859-1 (Latin-1) to UTF-8. The vendored .dst stores accented names
// (e.g. "CÔTE D'IVOIRE", "RÉUNION") as single Latin-1 bytes (0xD4, 0xC9), while
// the reference C dsdgen emits them as UTF-8. Latin-1 maps byte b directly to
// code point U+00b, so encoding each high byte as a rune reproduces dsdgen's
// UTF-8 output byte-for-byte.
func mustLoadCountries(file string) *StringValuesDistribution {
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
		d.values[0] = append(d.values[0], latin1ToUTF8(stripBackslashes(strings.TrimSpace(vals[0]))))

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

// latin1ToUTF8 reinterprets the bytes of s as ISO-8859-1 and returns the UTF-8
// encoding. Each byte b becomes the rune U+00b.
func latin1ToUTF8(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteRune(rune(s[i]))
	}

	return b.String()
}
