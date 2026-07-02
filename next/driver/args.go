package driver

import "math"

// Args is a reusable positional argument buffer for the hot bind path. Obtain
// one from Stmt.Bind, fill it with the typed setters in parameter order, then
// pass it to Conn.ExecWithArgs / QueryWithArgs / QueryRowWithArgs. The setters
// append into storage that is reused across iterations (Bind resets length,
// keeping capacity), so a warmed-up buffer binds without allocating — unlike
// the variadic Exec(ctx, stmt, args...) path, which allocates the "...any"
// slice on every call.
//
// Values are stored type-tagged and un-boxed: no value is placed into an "any"
// until a driver materialises the buffer for its wire protocol (see AppendTo).
// That boxing is the one allocation a real SQL driver still pays per bound
// value; it is unavoidable at any database/sql- or pgx-style boundary, which
// takes []any. The noop driver reads nothing and so binds fully allocation-free.
//
// Not safe for concurrent use; an Args belongs to a single Stmt on a single VU.
type Args struct {
	a []arg
}

type argKind uint8

const (
	argInt64 argKind = iota
	argFloat64
	argBool
	argBytes
	argString
	argNull
)

// arg is a value-typed argument cell: no pointers except the byte/string views,
// so appending one never boxes or heap-allocates within a warmed buffer.
type arg struct {
	kind argKind
	num  uint64 // int64 bits / float64 bits / bool
	b    []byte
	s    string
}

// Reset rewinds the buffer to empty, keeping its backing storage. Stmt.Bind
// calls this for you.
func (a *Args) Reset() *Args {
	a.a = a.a[:0]

	return a
}

// Len reports the number of bound arguments.
func (a *Args) Len() int { return len(a.a) }

// Int64 appends an int64 argument.
func (a *Args) Int64(v int64) *Args {
	a.a = append(a.a, arg{kind: argInt64, num: uint64(v)})

	return a
}

// Float64 appends a float64 argument.
func (a *Args) Float64(v float64) *Args {
	a.a = append(a.a, arg{kind: argFloat64, num: math.Float64bits(v)})

	return a
}

// Bool appends a bool argument.
func (a *Args) Bool(v bool) *Args {
	var n uint64
	if v {
		n = 1
	}

	a.a = append(a.a, arg{kind: argBool, num: n})

	return a
}

// Bytes appends a []byte argument. The slice is not copied; keep it valid until
// the call that consumes this Args returns.
func (a *Args) Bytes(v []byte) *Args {
	a.a = append(a.a, arg{kind: argBytes, b: v})

	return a
}

// String appends a string argument.
func (a *Args) String(v string) *Args {
	a.a = append(a.a, arg{kind: argString, s: v})

	return a
}

// Null appends a NULL argument.
func (a *Args) Null() *Args {
	a.a = append(a.a, arg{kind: argNull})

	return a
}

// AppendTo materialises the buffer into dst as the []any a SQL driver's wire
// layer takes, reusing dst's storage (dst is truncated first). This is where
// each value is boxed into an interface — the per-value allocation the hot path
// cannot escape at the driver boundary. Drivers keep one dst per connection and
// reuse it every call so only the boxing, never the slice, is paid again.
func (a *Args) AppendTo(dst []any) []any {
	dst = dst[:0]

	for i := range a.a {
		c := &a.a[i]

		switch c.kind {
		case argInt64:
			dst = append(dst, int64(c.num))
		case argFloat64:
			dst = append(dst, math.Float64frombits(c.num))
		case argBool:
			dst = append(dst, c.num != 0)
		case argBytes:
			dst = append(dst, c.b)
		case argString:
			dst = append(dst, c.s)
		case argNull:
			dst = append(dst, nil)
		}
	}

	return dst
}
