// Package sqlfile parses the "--+ section" / "--= query" SQL corpus format
// into named, parameterized queries at plan time, ready for positional
// binding on the hot path.
//
// Format (unchanged from v5's internal/static/parse_sql.ts, ported here):
//
//	--+ section_name
//	--= query_name
//	SELECT * FROM t WHERE id = :id;
//
// A line comment ("-- ...", checked after the "--+"/"--=" markers) is
// stripped before its query's text is stored; "/* ... */" block comments
// and "$$ ... $$" dollar-quoted bodies (procedure definitions) pass through
// verbatim, including any ";" they contain — a "--=" entry is never split
// on ";", matching v5.
//
// Named parameters ":name" are extracted in first-occurrence order and
// rewritten per PlaceholderStyle; see Query.Text.
package sqlfile
