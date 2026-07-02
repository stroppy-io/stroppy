package sqlfile

import (
	"errors"
	"strconv"
	"strings"
)

// PlaceholderStyle selects the positional-parameter syntax used when
// rewriting a query's ":name" markers for a specific SQL dialect.
type PlaceholderStyle int

const (
	// Dollar rewrites named parameters as $1, $2, ... (postgres, picodata).
	Dollar PlaceholderStyle = iota
	// Question rewrites named parameters as ? (mysql).
	Question
)

// scanError is rewriteParams' internal error type; it carries the byte
// offset into the scanned text so the caller can translate it to a source
// line.
type scanError struct {
	offset int
	err    error
}

func (e *scanError) Error() string { return e.err.Error() }
func (e *scanError) Unwrap() error { return e.err }

var (
	errUnterminatedString  = errors.New("unterminated string or quoted identifier")
	errUnterminatedComment = errors.New("unterminated /* */ comment")
	errUnterminatedDollar  = errors.New("unterminated $$ quoted body")
)

// rewriteParams walks text once, extracting ":name" parameter markers and
// producing both positional rewrites. A marker is only recognized outside
// string/identifier literals, "/* */" comments and "$$ ... $$" bodies, and
// only where it cannot be a postgres "::" cast: the character before ":"
// must be start-of-text, whitespace or "(", and the character(s) after the
// name must be whitespace, end-of-text, ";", ",", ")" or "::" — mirroring
// the boundary rule proven against this corpus by
// pkg/driver/sqldriver/run_query.go's argsRe in the v5 tree.
func rewriteParams(text string) (params []string, dollar, question string, err error) {
	var dollarB, questionB strings.Builder

	dollarB.Grow(len(text))
	questionB.Grow(len(text))

	index := make(map[string]int) // name -> first-occurrence index (Dollar slot + Params order)

	n := len(text)
	for i := 0; i < n; {
		c := text[i]

		switch {
		case c == '\'' || c == '"' || c == '`':
			end, ok := scanQuoted(text, i, c)
			if !ok {
				return nil, "", "", &scanError{i, errUnterminatedString}
			}

			dollarB.WriteString(text[i:end])
			questionB.WriteString(text[i:end])
			i = end

		case c == '/' && i+1 < n && text[i+1] == '*':
			end, ok := scanBlockComment(text, i)
			if !ok {
				return nil, "", "", &scanError{i, errUnterminatedComment}
			}

			dollarB.WriteString(text[i:end])
			questionB.WriteString(text[i:end])
			i = end

		case c == '$':
			end, isTag, ok := scanDollarQuote(text, i)
			if isTag && !ok {
				return nil, "", "", &scanError{i, errUnterminatedDollar}
			}

			if isTag {
				dollarB.WriteString(text[i:end])
				questionB.WriteString(text[i:end])
				i = end

				continue
			}

			dollarB.WriteByte(c)
			questionB.WriteByte(c)
			i++

		case c == ':':
			name, end, ok := scanParam(text, i)
			if !ok {
				dollarB.WriteByte(c)
				questionB.WriteByte(c)
				i++

				continue
			}

			idx, dup := index[name]
			if !dup {
				idx = len(index)
				index[name] = idx
				params = append(params, name)
			}

			dollarB.WriteString("$" + strconv.Itoa(idx+1))
			questionB.WriteByte('?')

			i = end

		default:
			dollarB.WriteByte(c)
			questionB.WriteByte(c)
			i++
		}
	}

	return params, dollarB.String(), questionB.String(), nil
}

// scanQuoted returns the index just past the closing quote matching text[start],
// honoring both doubled-quote (” / "" / “) and backslash escaping.
func scanQuoted(text string, start int, q byte) (int, bool) {
	n := len(text)
	for i := start + 1; i < n; i++ {
		switch text[i] {
		case '\\':
			if i+1 < n {
				i++
			}
		case q:
			if i+1 < n && text[i+1] == q {
				i++
				continue
			}

			return i + 1, true
		}
	}

	return 0, false
}

func scanBlockComment(text string, start int) (int, bool) {
	end := strings.Index(text[start+2:], "*/")
	if end < 0 {
		return 0, false
	}

	return start + 2 + end + 2, true
}

// scanDollarQuote recognizes a "$tag$" opening token at text[start] (tag may
// be empty, as in "$$") and looks for its matching close. isTag reports
// whether text[start] began a well-formed "$tag$" token at all; ok reports
// whether that token's close was found (only meaningful when isTag is true).
func scanDollarQuote(text string, start int) (end int, isTag, ok bool) {
	n := len(text)

	j := start + 1
	for j < n && (isAlnum(text[j]) || text[j] == '_') {
		j++
	}

	if j >= n || text[j] != '$' {
		return 0, false, false
	}

	tag := text[start : j+1]

	closeIdx := strings.Index(text[j+1:], tag)
	if closeIdx < 0 {
		return 0, true, false
	}

	return j + 1 + closeIdx + len(tag), true, true
}

// scanParam attempts to match a ":name" parameter at text[start] (text[start]
// == ':'). It returns the parameter name and the index just past it — never
// including the trailing boundary character, so back-to-back parameters
// separated by a single space are both recognized (the shared boundary
// character is only peeked, not consumed).
func scanParam(text string, start int) (name string, end int, ok bool) {
	if start > 0 {
		p := text[start-1]
		if p != ' ' && p != '\t' && p != '\n' && p != '\r' && p != '(' {
			return "", 0, false
		}
	}

	n := len(text)

	i := start + 1
	if i >= n || !isNameStart(text[i]) {
		return "", 0, false
	}

	j := i + 1
	for j < n && isAlnum(text[j]) {
		j++
	}

	if j < n {
		next := text[j]
		if next == ':' {
			if j+1 >= n || text[j+1] != ':' {
				return "", 0, false
			}
		} else if next != ' ' && next != '\t' && next != '\n' && next != '\r' &&
			next != ';' && next != ',' && next != ')' {
			return "", 0, false
		}
	}

	return text[i:j], j, true
}

func isNameStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isAlnum(b byte) bool {
	return isNameStart(b) || (b >= '0' && b <= '9')
}
