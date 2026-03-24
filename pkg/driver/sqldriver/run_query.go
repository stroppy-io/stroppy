package sqldriver

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/sqldriver/queries"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
)

var (
	ErrMissedArgument = errors.New("missed arguments present")
	ErrExtraArgument  = errors.New("extra arguments provided")
	// TODO: synchronize with re from TS parse_sql.ts
	argsRe = regexp.MustCompile(`(\s|^|\()(:[a-zA-Z0-9_]+)(\s|$|;|::|,|\))`)
)

// RunQuery executes sql with named :arg placeholders and returns rows cursor.
func RunQuery[R any](
	ctx context.Context,
	db QueryContext[R],
	wrapRows func(R) driver.Rows,
	dialect queries.Dialect,
	lg *zap.Logger,
	sqlStr string,
	args map[string]any,
) (*driver.QueryResult, error) {
	processedSQL, argsArr, err := ProcessArgs(dialect, sqlStr, args)
	if err != nil {
		if errors.Is(err, ErrExtraArgument) {
			lg.Warn(err.Error(), zap.String("sql", sqlStr))
		} else {
			lg.Error("arguments processing error", zap.String("sql", sqlStr), zap.Error(err))

			return nil, fmt.Errorf("arguments processing error: %w", err)
		}
	}

	start := time.Now()
	rawRows, err := db.QueryContext(ctx, processedSQL, argsArr...)
	elapsed := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("failed to execute sql: %w", err)
	}

	return &driver.QueryResult{
		Stats: &stats.Query{Elapsed: elapsed},
		Rows:  wrapRows(rawRows),
	}, nil
}

// ProcessArgs takes sql which contains ":arg" marks and args map "arg" -> value.
// It returns sql where all marks are replaced with dialect-specific placeholders,
// and an array of any containing the arguments in the right order.
// When dialect.Deduplicate() is true, repeated named parameters share a single
// positional placeholder (e.g. pgx's $1 back-references).
func ProcessArgs(
	dialect queries.Dialect, sqlStr string, args map[string]any,
) (newSQL string, argsArr []any, err error) {
	var (
		resultArgs []any
		missedArgs []string
	)

	dedup := dialect.Deduplicate()
	seenArgs := make(map[string]int) // argName → assigned paramCounter (for dedup reuse & extra-arg detection)

	var sb strings.Builder

	lastIndex := 0
	paramCounter := 0

	// match[0:1] covers the full regex match including surrounding whitespace/punctuation;
	// match[4:5] is capture group 2 — the ":argName" token itself.
	matches := argsRe.FindAllStringSubmatchIndex(sqlStr, -1)

	for _, match := range matches {
		fullEnd := match[1]
		argStart, argEnd := match[4], match[5]

		rawArg := sqlStr[argStart:argEnd] // e.g. ":id"
		argName := rawArg[1:]             // strip leading ":"

		// copy everything before the ":arg" token (including any leading punctuation inside the match)
		sb.WriteString(sqlStr[lastIndex:argStart])

		if val, ok := args[argName]; ok {
			idx, seen := seenArgs[argName]
			if dedup && seen {
				// reuse the placeholder index assigned on the first occurrence
				sb.WriteString(dialect.Placeholder(idx))
			} else {
				// first occurrence: assign the next positional placeholder
				sb.WriteString(dialect.Placeholder(paramCounter))
				seenArgs[argName] = paramCounter

				resultArgs = append(resultArgs, val)
				paramCounter++
			}
		} else {
			// arg referenced in SQL but not supplied — keep the raw token and report it
			if !slices.Contains(missedArgs, argName) {
				missedArgs = append(missedArgs, argName)
			}

			sb.WriteString(rawArg)
		}

		// copy trailing punctuation that belongs to the full match (e.g. closing paren, comma)
		sb.WriteString(sqlStr[argEnd:fullEnd])

		lastIndex = fullEnd
	}

	// copy the remainder of the SQL after the last match
	sb.WriteString(sqlStr[lastIndex:])

	if len(missedArgs) > 0 {
		return "", nil, fmt.Errorf("%w: [%s]", ErrMissedArgument, strings.Join(missedArgs, ", "))
	}

	// args supplied by the caller but never referenced in the SQL
	if len(seenArgs) < len(args) {
		var diff []string

		for k := range args {
			if _, ok := seenArgs[k]; !ok {
				diff = append(diff, k)
			}
		}

		return sb.String(), resultArgs, fmt.Errorf(
			"%w: [%s]",
			ErrExtraArgument,
			strings.Join(diff, ", "),
		)
	}

	return sb.String(), resultArgs, nil
}
