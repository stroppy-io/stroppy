package driver

// InsertMethod selects how a [Conn] drains a [*mem.RowBuf] into a table. The
// method is advisory per call: a driver maps [InsertNative] to its own fastest
// path and implements the rest where the backend allows; a method a driver
// cannot serve is an explicit error rather than a silent fallback.
//
// The fill-batch-flush cadence (how big the buffer grows before a flush) stays
// the caller's job — [github.com/stroppy-io/stroppy/next/bench.Loader.Spec.Batch].
// The method only changes how one filled buffer is drained.
type InsertMethod uint8

const (
	// InsertNative is the zero value: the driver picks its fastest path (pg =
	// COPY, mysql = multi-row bulk, …). It is a selector the driver resolves,
	// not a real drain path.
	InsertNative InsertMethod = iota
	// InsertPlainQuery emits one parameterized INSERT per row. Slowest path;
	// exists for theory-checking — measuring per-row overhead against the bulk
	// paths rather than loading fast.
	InsertPlainQuery
	// InsertPlainBulk builds one multi-row VALUES INSERT per (sub)batch. The
	// portable fast path across drivers that lack a COPY equivalent.
	InsertPlainBulk
	// InsertColumnar is the backend-specific columnar path (pg COPY FROM,
	// mysql JSON_TABLE, ydb NATIVE). Opt-in; not every backend has one.
	InsertColumnar
)

// String renders the v5/SDK insert-method name.
func (m InsertMethod) String() string {
	switch m {
	case InsertNative:
		return "native"
	case InsertPlainQuery:
		return "plain_query"
	case InsertPlainBulk:
		return "plain_bulk"
	case InsertColumnar:
		return "columnar"
	default:
		return "unknown"
	}
}

// ParseInsertMethod maps an insert-method name (as produced by
// [InsertMethod.String]) to its value. It is the single authority for the
// name→method direction so a test's insert-method knob resolves through the
// SDK rather than each caller re-rolling the switch. An unrecognized name
// reports ok=false; the zero value ([InsertNative]) leaves the driver to pick.
func ParseInsertMethod(name string) (InsertMethod, bool) {
	switch name {
	case InsertNative.String():
		return InsertNative, true
	case InsertPlainQuery.String():
		return InsertPlainQuery, true
	case InsertPlainBulk.String():
		return InsertPlainBulk, true
	case InsertColumnar.String():
		return InsertColumnar, true
	default:
		return 0, false
	}
}

// InsertDefaulter is implemented by drivers whose slot carries a resolved
// default [InsertMethod] an unset caller inherits. It mirrors
// [DefaultIsolationer]: the bench layer resolves a caller's zero ([InsertNative],
// "driver picks") through it so an operator's --insert.method override reaches
// a step that did not pin its own method. Drivers without a configured default
// do not implement it; the caller's [InsertNative] then passes straight through
// and the driver maps it to its own best.
type InsertDefaulter interface {
	DefaultInsertMethod() InsertMethod
}
