package bench

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

const maxErrorSampleBytes = 512

var (
	errorSampleQuoted = regexp.MustCompile("\"(?:[^\"\\\\]|\\\\.)*\"|'(?:[^'\\\\]|\\\\.)*'|`[^`]*`")
	errorSampleSQL    = regexp.MustCompile(`(?is)\b(SELECT\s+|(?:INSERT|REPLACE)\s+INTO\s+|UPDATE\s+\S+\s+SET\s+|` +
		`DELETE\s+FROM\s+|CALL\s+|CREATE\s+TABLE\s+|ALTER\s+TABLE\s+|DROP\s+TABLE\s+|WITH\s+\S+\s+AS\s*\().*`)
	errorSampleArguments = regexp.MustCompile(`(?is)\b(args|arguments|parameters)\s*[:=\[]?.*`)
)

func backendErrorSample(err error) (code, sample string) {
	if err == nil {
		return "", ""
	}

	if backend, ok := errors.AsType[*pgconn.PgError](err); ok {
		return backend.Code, backend.Message
	}

	if backend, ok := errors.AsType[*mysql.MySQLError](err); ok {
		return strconv.FormatUint(uint64(backend.Number), 10), backend.Message
	}

	for {
		if cause := errors.Unwrap(err); cause != nil {
			err = cause

			continue
		}

		if joined, ok := err.(interface{ Unwrap() []error }); ok && len(joined.Unwrap()) > 0 {
			err = joined.Unwrap()[0]

			continue
		}

		return "", err.Error()
	}
}

func representativeError(err error, redactor *strings.Replacer) (code, sample string) {
	code, sample = backendErrorSample(err)

	if redactor != nil {
		redacted := redactor.Replace(sample)
		if redacted != sample && errorSampleSQL.MatchString(sample) {
			redacted = "<SQL omitted>"
		}

		sample = redacted
	}

	sample = errorSampleQuoted.ReplaceAllString(sample, "<redacted>")
	sample = errorSampleArguments.ReplaceAllString(sample, "<arguments omitted>")
	sample = errorSampleSQL.ReplaceAllString(sample, "<SQL omitted>")
	sample = strings.ToValidUTF8(sample, "?")

	sample = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}

		return r
	}, sample)
	if len(sample) > maxErrorSampleBytes {
		sample = sample[:maxErrorSampleBytes]
		for !utf8.ValidString(sample) {
			sample = sample[:len(sample)-1]
		}
	}

	return code, strings.TrimSpace(sample)
}
