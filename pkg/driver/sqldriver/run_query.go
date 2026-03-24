package sqldriver

import (
	"context"
	"errors"
	"fmt"
	"maps"
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
	seenMissed := make(map[string]bool)

	var sb strings.Builder

	lastIndex := 0
	paramCounter := 0

	matches := argsRe.FindAllStringSubmatchIndex(sqlStr, -1)

	for _, match := range matches {
		fullStart, fullEnd := match[0], match[1]
		argStart, argEnd := match[4], match[5]

		sb.WriteString(sqlStr[lastIndex:fullStart])
		sb.WriteString(sqlStr[fullStart:argStart])

		rawArg := sqlStr[argStart:argEnd]
		argName := rawArg[1:]

		if val, ok := args[argName]; ok {
			if dedup {
				if idx, seen := seenArgs[argName]; seen {
					sb.WriteString(dialect.Placeholder(idx))
				} else {
					sb.WriteString(dialect.Placeholder(paramCounter))
					seenArgs[argName] = paramCounter
					resultArgs = append(resultArgs, val)
					paramCounter++
				}
			} else {
				sb.WriteString(dialect.Placeholder(paramCounter))
				seenArgs[argName] = paramCounter
				resultArgs = append(resultArgs, val)
				paramCounter++
			}
		} else {
			if !seenMissed[argName] {
				missedArgs = append(missedArgs, argName)
				seenMissed[argName] = true
			}

			sb.WriteString(rawArg)
		}

		sb.WriteString(sqlStr[argEnd:fullEnd])

		lastIndex = fullEnd
	}

	sb.WriteString(sqlStr[lastIndex:])

	if len(missedArgs) > 0 {
		return "", nil, fmt.Errorf("%w: [%s]", ErrMissedArgument, strings.Join(missedArgs, ", "))
	}

	if len(seenArgs) < len(args) {
		diff := sliceDifference(
			slices.Collect(maps.Keys(seenArgs)),
			slices.Collect(maps.Keys(args)),
		)

		return sb.String(), resultArgs, fmt.Errorf(
			"%w: [%s]",
			ErrExtraArgument,
			strings.Join(diff, ", "),
		)
	}

	return sb.String(), resultArgs, nil
}

func sliceDifference[T comparable](subset, full []T) []T {
	var diff []T

	for _, v := range full {
		if !slices.Contains(subset, v) {
			diff = append(diff, v)
		}
	}

	return diff
}
