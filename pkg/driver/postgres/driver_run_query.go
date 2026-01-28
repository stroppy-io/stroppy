package postgres

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// RunQuery exucetse sql with args in form :arg.
func (d *Driver) RunQuery(
	ctx context.Context,
	sql string,
	args map[string]any,
) (*stroppy.DriverQueryStat, error) {
	processedSQL, argsArr, err := processArgs(sql, args)
	if err != nil {
		if errors.Is(err, ErrExtraArgument) {
			d.logger.Warn(err.Error(), zap.String("sql", sql))
		} else {
			d.logger.Error("arguments processing error", zap.String("sql", sql), zap.Error(err))

			return nil, fmt.Errorf("arguments processing error: %w", err)
		}
	}

	_, _ = d.pgxPool.Exec(ctx, processedSQL, argsArr...)

	return &stroppy.DriverQueryStat{}, nil
}

var (
	ErrMissedArgument = errors.New("missed arguments present")
	ErrExtraArgument  = errors.New("extra arguments provided")
	// TODO: syncronize with re from TS parse_sql.ts
	argsRe = regexp.MustCompile(`(\s|^|\()(:[a-zA-Z0-9_]+)(\s|$|;|::|,|\))`)
)

// processArgs takse sql which contains ":arg" marks
// and args map "arg" -> value.
// It returns sql where all the maks replaced with postgresql placeholders like "$1",
// and array of any, which contains the arguments in the right order.
// If sql contains marks that is'n present in args map - there is an error
// errors.Is(err, ErrMissedArgument) and text contains info about all missed arguments.
func processArgs(sql string, args map[string]any) (newSQL string, argsArr []any, err error) {
	var (
		resultArgs []any
		missedArgs []string
	)
	// Use a map to avoid duplicate entries in the error message
	seenSuccesToArgNumber := make(map[string]int)
	seenMissed := make(map[string]bool)

	var sb strings.Builder

	lastIndex := 0
	paramCounter := 1

	// FindAllStringSubmatchIndex returns a slice of index pairs identifying matches and submatches.
	// match[0], match[1] -> indices of the full match (including spaces)
	// match[2], match[3] -> indices of the first submatch (the :arg part)
	matches := argsRe.FindAllStringSubmatchIndex(sql, -1)

	for _, match := range matches {
		fullStart, fullEnd := match[0], match[1]
		argStart, argEnd := match[4], match[5]

		// 1. Append the text of the query that appears before this match
		sb.WriteString(sql[lastIndex:fullStart])

		// 2. Append the specific whitespace character(s) that appeared before the argument
		// (e.g., a newline or a specific indentation tab)
		sb.WriteString(sql[fullStart:argStart])

		// 3. Extract the argument name (remove the leading ':')
		rawArg := sql[argStart:argEnd] // e.g. ":userId"
		argName := rawArg[1:]          // e.g. "userId"

		// 4. Look up the argument
		if val, ok := args[argName]; ok {
			// Append the Postgres placeholder
			// Add counter if it's a first occurrence
			if oldIndex := seenSuccesToArgNumber[argName]; oldIndex == 0 {
				sb.WriteString("$" + strconv.Itoa(paramCounter))
				seenSuccesToArgNumber[argName] = paramCounter

				resultArgs = append(resultArgs, val)
				paramCounter++
			} else { // reuse old index if argument seen alredy
				sb.WriteString("$" + strconv.Itoa(oldIndex))
			}
		} else {
			// Track missing arguments
			if !seenMissed[argName] {
				missedArgs = append(missedArgs, argName)
				seenMissed[argName] = true
			}
			// In case of error, we technically fail, but for the string construction
			// we can leave the original placeholder or write an invalid marker.
			// Here we write the original back to keep the string structure.
			sb.WriteString(rawArg)
		}

		// 5. Append the specific whitespace character(s) that appeared after the argument
		sb.WriteString(sql[argEnd:fullEnd])

		lastIndex = fullEnd
	}

	// Append any remaining part of the query
	sb.WriteString(sql[lastIndex:])

	// If there were missing arguments, return the error
	if len(missedArgs) > 0 {
		return "", nil, fmt.Errorf("%w: [%s]", ErrMissedArgument, strings.Join(missedArgs, ", "))
	}

	if len(resultArgs) < len(args) {
		diff := sliceDifference(
			slices.Collect(maps.Keys(seenSuccesToArgNumber)),
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
