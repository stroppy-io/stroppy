package driver

import "math"

// Args is a reusable argument buffer for the hot bind path. Obtain one from
// Stmt.Bind, fill it with the typed setters, then pass it to Conn.ExecWithArgs /
// QueryWithArgs / QueryRowWithArgs.
//
// Two bind modes share one buffer:
//
//   - Positional (append): the typed setters Int64/Float64/Bool/Bytes/String/Null
//     append cells in parameter order. This is the original variadic-replacement
//     path; reset rewinds length to zero, keeping capacity, so a warmed buffer
//     binds allocation-free.
//   - Named (by index): SetInt64/SetFloat64/.../Set write a cell at the index a
//     name resolves to. The name->index map is built cold, once, when the query
//     handle is prepared (the parsed :param query gives the parameter order);
//     the hot path is a single map lookup + an index write, with no per-call
//     append or name-parse. Reset fills the pre-sized buffer with NULL cells so
//     every named cell has a defined value even when a setter is skipped.
//
// Match the mode to the query: a [Stmt] whose query carries :params is prepared
// in named mode (use the Set* setters); a Stmt with no params stays positional
// (use Int64 et al). Mixing the two on one buffer is undefined.
//
// Values are stored type-tagged and un-boxed: no value is placed into an "any"
// until a driver materialises the buffer for its wire protocol (see AppendTo).
// That boxing is the one allocation a real SQL driver still pays per bound value
// at any database/sql- or pgx-style boundary, which takes []any; it is
// unavoidable and identical under either bind mode. The noop driver reads
// nothing and so binds fully allocation-free.
//
// Not safe for concurrent use; an Args belongs to a single Stmt on a single VU.
type Args struct {
	a     []arg
	names map[string]int // optional name->index; nil selects positional mode
	n     int            // intended cell count in named mode (len(names))
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
// so writing one never boxes or heap-allocates within a warmed buffer.
type arg struct {
	kind argKind
	num  uint64 // int64 bits / float64 bits / bool
	b    []byte
	s    string
}

// BuildNameIndex builds a name->positional-index map from a query's named
// parameters (as returned by sqlfile.Query.Params), in first-occurrence order
// with duplicates removed — matching the dollar-style $n back-reference rule so
// a name recurring in the SQL maps to one index. Stmt implementations call this
// once at Prepare and hand the result to [Args.SetNames].
func BuildNameIndex(params []string) map[string]int {
	m := make(map[string]int, len(params))
	for i, p := range params {
		if _, exists := m[p]; !exists {
			m[p] = i
		}
	}
	return m
}

// SetNames associates a name->index map with this buffer and switches it into
// named-bind mode: the buffer is pre-sized to len(m) and Reset fills it with
// NULL cells so named setters write by index without growing the slice. The
// map is consulted by the Set/SetInt64/... setters; the positional setters
// ignore it. Stmt implementations call this once at Prepare; callers should
// match the bind mode to the query (named setters for a query with :params,
// positional setters otherwise). Passing nil keeps positional mode.
func (a *Args) SetNames(m map[string]int) {
	a.names = m
	a.n = len(m)
	if a.n > cap(a.a) {
		a.a = make([]arg, a.n)
	} else if a.n > len(a.a) {
		a.a = a.a[:a.n]
	}
}

// Reset rewinds the buffer for the next bind. In named mode it truncates back to
// the intended cell count (so a stray positional append cannot grow the buffer
// across iterations) and marks every cell NULL; named setters overwrite by
// index. In positional mode it truncates to length zero; positional setters
// append. Stmt.Bind calls this for you.
func (a *Args) Reset() *Args {
	if a.names != nil {
		a.a = a.a[:a.n]
		for i := range a.a {
			a.a[i] = arg{kind: argNull}
		}
		return a
	}
	a.a = a.a[:0]
	return a
}

// Len reports the number of bound arguments: the pre-sized cell count in named
// mode, the appended count in positional mode.
func (a *Args) Len() int { return len(a.a) }

// Int64 appends an int64 argument (positional mode).
func (a *Args) Int64(v int64) *Args {
	a.a = append(a.a, arg{kind: argInt64, num: uint64(v)})
	return a
}

// Float64 appends a float64 argument (positional mode).
func (a *Args) Float64(v float64) *Args {
	a.a = append(a.a, arg{kind: argFloat64, num: math.Float64bits(v)})
	return a
}

// Bool appends a bool argument (positional mode).
func (a *Args) Bool(v bool) *Args {
	var n uint64
	if v {
		n = 1
	}
	a.a = append(a.a, arg{kind: argBool, num: n})
	return a
}

// Bytes appends a []byte argument (positional mode). The slice is not copied;
// keep it valid until the call that consumes this Args returns.
func (a *Args) Bytes(v []byte) *Args {
	a.a = append(a.a, arg{kind: argBytes, b: v})
	return a
}

// String appends a string argument (positional mode).
func (a *Args) String(v string) *Args {
	a.a = append(a.a, arg{kind: argString, s: v})
	return a
}

// Null appends a NULL argument (positional mode).
func (a *Args) Null() *Args {
	a.a = append(a.a, arg{kind: argNull})
	return a
}

// SetInt64 binds v to the named parameter (named mode). Allocation-free on the
// hot path: one map lookup + one index write.
func (a *Args) SetInt64(name string, v int64) *Args {
	a.a[a.names[name]] = arg{kind: argInt64, num: uint64(v)}
	return a
}

// SetFloat64 binds v to the named parameter (named mode).
func (a *Args) SetFloat64(name string, v float64) *Args {
	a.a[a.names[name]] = arg{kind: argFloat64, num: math.Float64bits(v)}
	return a
}

// SetBool binds v to the named parameter (named mode).
func (a *Args) SetBool(name string, v bool) *Args {
	var n uint64
	if v {
		n = 1
	}
	a.a[a.names[name]] = arg{kind: argBool, num: n}
	return a
}

// SetBytes binds v to the named parameter (named mode). The slice is not copied;
// keep it valid until the call that consumes this Args returns.
func (a *Args) SetBytes(name string, v []byte) *Args {
	a.a[a.names[name]] = arg{kind: argBytes, b: v}
	return a
}

// SetString binds v to the named parameter (named mode).
func (a *Args) SetString(name string, v string) *Args {
	a.a[a.names[name]] = arg{kind: argString, s: v}
	return a
}

// SetNull binds NULL to the named parameter (named mode).
func (a *Args) SetNull(name string) *Args {
	a.a[a.names[name]] = arg{kind: argNull}
	return a
}

// Set binds v to the named parameter (named mode), dispatching on v's concrete
// type to the matching typed cell. The supported types are exactly those of the
// typed setters — int64, float64, bool, []byte, string and nil (NULL); any other
// type leaves the cell at its Reset default of NULL. The typed SetInt64/...
// setters are the allocation-free form; Set is the convenience mirror that costs
// one interface boxing at the call site and so is not the recommended hot-path
// form.
func (a *Args) Set(name string, v any) *Args {
	idx := a.names[name]
	switch x := v.(type) {
	case int64:
		a.a[idx] = arg{kind: argInt64, num: uint64(x)}
	case float64:
		a.a[idx] = arg{kind: argFloat64, num: math.Float64bits(x)}
	case bool:
		var n uint64
		if x {
			n = 1
		}
		a.a[idx] = arg{kind: argBool, num: n}
	case []byte:
		a.a[idx] = arg{kind: argBytes, b: x}
	case string:
		a.a[idx] = arg{kind: argString, s: x}
	case nil:
		a.a[idx] = arg{kind: argNull}
	}
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
