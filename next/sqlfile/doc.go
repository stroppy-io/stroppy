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
// Named parameters are the author-facing placeholder: write ":name". The
// dbdrv renders them to indexed ("$1") or positional ("?") at Prepare time
// (see [Query.Text] and [PlaceholderStyle]); that rendering is a driver
// detail, never the author's concern.
package sqlfile
