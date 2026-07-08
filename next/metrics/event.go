package metrics

// Stage-1 telemetry substrate (D6). This file freezes the EventRow schema up
// front so stage-2 (sampled Tier-1 event rows + live log/trace viewers) and
// stage-3 (Tier-2 blob capture) build on it without redesign. No rows flow in
// stage-1: only aggregates (counters/histograms, see [Registry]/[Shard]) and
// the Report projections (see [Sink]) are live. The schema is committed now
// because the column set constrains every later tier — declaring it once keeps
// the vocabulary stable across the staging.
//
// The design follows the D6 core distinction: an aggregate update is not an
// event row. Aggregates fold in place at the source (8 bytes, ~1ns, never
// sampled). An event row is a separate, deliberate ~500 B write done only when
// the detail — not just the count — is wanted; rows are the only stream that
// is sampled (Tier 1) or fully captured (Tier 2).
//
// Three column kinds (D6), all resolved by Freeze so the hot path stays 0-alloc:
//   - Numeric aggregate: counters/histograms, never produce a row.
//   - Fixed tag (declared enum): the "known-ahead signal" — step, tx, table.
//     Interned at Freeze to a small int id ([StageTagID]); the hot path writes
//     the id, never the string.
//   - Dynamic interned string: the "unknown string" case (DB error text,
//     SQLSTATE). A run-scoped intern table copies a new string once and hands
//     out a [StageStringID]; later occurrences are 0-alloc id writes.

// EventKind tags the purpose of one event row. Tier-1 samplers and the tier-2
// blob reader dispatch on it; the log/trace viewers group by it. The set is
// closed for stage-1 (no rows flow); new kinds arrive with the tiers that emit
// them.
type EventKind uint8

const (
	// EventOp is one driver operation: a query, exec, or COPY. The trace view's
	// atom; sampled in Tier 1.
	EventOp EventKind = iota + 1
	// EventTx is one transaction's terminal outcome (committed/rolled-back) with
	// its whole-tx wall-clock duration.
	EventTx
	// EventTransition is a DAG lifecycle transition (step start/finish, run
	// end). Always emitted richly — inherently low-frequency.
	EventTransition
	// EventLog is an author-emitted structured log line, the log view's atom.
	EventLog
)

// StageTagID is the interned id of one fixed-tag enum value (e.g. the tx name
// "new_order"). The Registry assigns ids at Freeze; the hot path writes the id
// in place of the string. Zero means "unset".
type StageTagID uint16

// StageStringID is the interned id of one dynamic string (DB error text,
// SQLSTATE). A run-scoped intern table holds the bytes; the hot path writes the
// id. Zero means "none". Resolution id -> text happens off the hot path in a
// viewer.
type StageStringID uint32

// TagCol declares one fixed-tag column of the event schema. The value space is
// the author-declared enum (e.g. the five TPC-C tx names) collected at Freeze
// from author [Tag] declarations on instruments. Each declared value takes one
// StageTagID; the column's Name is what a viewer groups by ("tx", "table").
//
// A TagCol is event-purpose metadata only in stage-1: it records what columns
// future event rows will carry. Aggregates (the live tier-0 path) consume the
// same enum via [Instrument] tags; the TagCol here is the row-schema mirror.
type TagCol struct {
	Name   string   // column name: "tx", "table", "step"
	Values []string // declared enum values, in declaration order
}

// EventRow is one Tier-1/Tier-2 event row (D6). System columns are present on
// every row; tag columns are the author-declared fixed-tag enums; dynamic
// strings carry an intern id the viewer resolves. Fixed-width, columnar,
// 0-alloc to write: every field is a value type.
//
// Stage-1 freezes the schema; no EventRow is constructed until stage-2 lands
// the sampled Tier-1 ring buffer. The struct is exported now so the columnar
// contract is visible and stable.
type EventRow struct {
	// System columns.
	TS    int64         // unix-nano timestamp
	VU    int           // worker index
	Cycle uint64        // iteration cursor
	Step  StageTagID    // owning step
	Kind  EventKind     // event purpose
	Dur   int64         // nanoseconds; 0 for non-duration events

	// Fixed-tag columns (author-declared enums). Resolved to ids at Freeze.
	Tx    StageTagID    // transaction name (Tag("tx"))
	Table StageTagID    // table name (Tag("table"))

	// Dynamic-string column (DB error text, SQLSTATE). Intern id; 0 = none.
	Err StageStringID
}

// EventSchema is the frozen event-row schema: the system columns (implicit,
// always present) plus the author-declared tag columns collected from
// instrument declarations. Built once at Freeze; read by viewers and the tier-2
// blob writer.
//
// Stage-1 builds the schema for inspection/projection but does not yet emit
// rows; stage-2's Tier-1 sampler is the first writer.
type EventSchema struct {
	// Tags is the set of declared fixed-tag columns, in declaration order. Each
	// column's Values are the interned enum seen at Freeze.
	Tags []TagCol
}
