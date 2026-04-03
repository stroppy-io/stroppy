package sqldriver

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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

// parsedQuery is the dialect-specific, value-independent decomposition of a SQL template.
// It is computed once per unique (dialect, sqlStr) pair and reused across all calls.
type parsedQuery struct {
	processedSQL   string
	argOrder       []string            // positional slot → arg name; may repeat when dedup=false
	referencedArgs map[string]struct{} // unique set of arg names for extra-arg detection
}

// sqlCache stores *parsedQuery values keyed by cacheKey(dialect, sqlStr).
// sync.Map is used because the load-test workload is read-heavy after warmup.
var (
	sqlCache     sync.Map
	sqlCacheSize atomic.Int32
)

const sqlCacheMaxSize = 1000

// lookupOrParse returns the cached parsedQuery for the given (dialect, sql) pair,
// parsing and caching it on the first call. New entries are rejected once the
// cache reaches sqlCacheMaxSize to guard against unbounded growth from
// dynamically constructed SQL strings.
func lookupOrParse(dialect queries.Dialect, sqlStr string) *parsedQuery {
	key := dialect.Placeholder(0) + "|" + sqlStr

	if v, ok := sqlCache.Load(key); ok {
		return v.(*parsedQuery) //nolint:errcheck,forcetypeassert // map stores only *parsedQuery values
	}

	pq := parseQueryTemplate(dialect, sqlStr)

	if sqlCacheSize.Load() < sqlCacheMaxSize {
		if _, loaded := sqlCache.LoadOrStore(key, pq); !loaded {
			sqlCacheSize.Add(1)
		}
	}

	return pq
}

// parseQueryTemplate runs the expensive parse phase: regex + string building.
// The result is dialect-specific but value-independent and safe to cache.
func parseQueryTemplate(dialect queries.Dialect, sqlStr string) *parsedQuery {
	dedup := dialect.Deduplicate()
	seenArgs := make(map[string]int) // argName → first paramCounter assigned
	referencedArgs := make(map[string]struct{})

	var argOrder []string

	var sb strings.Builder

	lastIndex := 0
	paramCounter := 0

	for _, match := range argsRe.FindAllStringSubmatchIndex(sqlStr, -1) {
		fullEnd := match[1]
		argStart, argEnd := match[4], match[5]

		argName := sqlStr[argStart+1 : argEnd] // strip leading ":"

		sb.WriteString(sqlStr[lastIndex:argStart])

		idx, seen := seenArgs[argName]
		if dedup && seen {
			sb.WriteString(dialect.Placeholder(idx))
		} else {
			sb.WriteString(dialect.Placeholder(paramCounter))
			seenArgs[argName] = paramCounter
			argOrder = append(argOrder, argName)
			paramCounter++
		}

		referencedArgs[argName] = struct{}{}

		sb.WriteString(sqlStr[argEnd:fullEnd])
		lastIndex = fullEnd
	}

	sb.WriteString(sqlStr[lastIndex:])

	return &parsedQuery{
		processedSQL:   sb.String(),
		argOrder:       argOrder,
		referencedArgs: referencedArgs,
	}
}

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
//
// The regex + string-building parse phase is cached per (dialect, sqlStr) pair
// so that repeated calls with the same template pay only a map lookup.
func ProcessArgs(
	dialect queries.Dialect, sqlStr string, args map[string]any,
) (newSQL string, argsArr []any, err error) {
	pq := lookupOrParse(dialect, sqlStr)

	// fill phase: build argsArr from args using cached argOrder
	var argsSlice []any
	if len(pq.argOrder) > 0 {
		argsSlice = make([]any, len(pq.argOrder))
	}

	var missedArgs []string

	for i, name := range pq.argOrder {
		if val, ok := args[name]; ok {
			argsSlice[i] = val
		} else if !slices.Contains(missedArgs, name) {
			missedArgs = append(missedArgs, name)
		}
	}

	if len(missedArgs) > 0 {
		return "", nil, fmt.Errorf("%w: [%s]", ErrMissedArgument, strings.Join(missedArgs, ", "))
	}

	// args supplied by the caller but never referenced in the SQL
	if len(args) > len(pq.referencedArgs) {
		var diff []string

		for k := range args {
			if _, ok := pq.referencedArgs[k]; !ok {
				diff = append(diff, k)
			}
		}

		sort.Strings(diff)

		return pq.processedSQL, argsSlice, fmt.Errorf(
			"%w: [%s]",
			ErrExtraArgument,
			strings.Join(diff, ", "),
		)
	}

	return pq.processedSQL, argsSlice, nil
}
