//go:build integration

package integration

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
	"github.com/stroppy-io/stroppy/pkg/datagen/runtime"
)

// Parent population "parents" has 10 entities; each parent contributes a
// fixed number of "children" rows. The spec exercises the relationship
// runtime end-to-end: LookupPop compilation for the outer side, nested
// ENTITY/LINE iteration for the inner side, and Lookup expressions that
// pull parent attrs across the relationship boundary.
const (
	childParentCount  int64 = 10
	childDegree       int64 = 3
	childRowCount           = childParentCount * childDegree
	childParentPop          = "parents"
	childIterPop            = "children"
	childRelationship       = "parent_child"
)

// childColumns is the emission order for the children table; callers
// must supply the same order to CopyFrom and to SELECT reads.
var childColumns = []string{"c_id", "c_parent_id", "c_line", "c_label"}

// childSpec builds the InsertSpec exercised by the test. The outer
// parent population is declared as a LookupPop so its attrs are
// evaluable via Lookup; the inner children population is the one this
// spec iterates and inserts.
//
// Attrs:
//
//	c_id        = rowIndex(GLOBAL) + 1                       -> 1..30
//	c_parent_id = Lookup("parents", "p_id", rowIndex(ENTITY)) -> 1..10 FK
//	c_line      = rowIndex(LINE) + 1                          -> 1..3
//	c_label     = std.format("%s-%d",
//	                Lookup("parents","p_label",rowIndex(ENTITY)),
//	                rowIndex(LINE)+1)                         -> "Pnnn-i"
func childSpec() *dgproto.InsertSpec {
	parents := &dgproto.LookupPop{
		Population: &dgproto.Population{Name: childParentPop, Size: childParentCount},
		Attrs: []*dgproto.Attr{
			attrOf("p_id", binOpOf(dgproto.BinOp_ADD, rowIndexKind(dgproto.RowIndex_ENTITY), litOf(int64(1)))),
			attrOf("p_label", callOf("std.format", litOf("P%03d"),
				binOpOf(dgproto.BinOp_ADD, rowIndexKind(dgproto.RowIndex_ENTITY), litOf(int64(1))))),
		},
		ColumnOrder: []string{"p_id", "p_label"},
	}

	attrs := []*dgproto.Attr{
		attrOf("c_id", binOpOf(dgproto.BinOp_ADD, rowIndexKind(dgproto.RowIndex_GLOBAL), litOf(int64(1)))),
		attrOf("c_parent_id", lookupOf(childParentPop, "p_id", rowIndexKind(dgproto.RowIndex_ENTITY))),
		attrOf("c_line", binOpOf(dgproto.BinOp_ADD, rowIndexKind(dgproto.RowIndex_LINE), litOf(int64(1)))),
		attrOf("c_label", callOf("std.format", litOf("%s-%d"),
			lookupOf(childParentPop, "p_label", rowIndexKind(dgproto.RowIndex_ENTITY)),
			binOpOf(dgproto.BinOp_ADD, rowIndexKind(dgproto.RowIndex_LINE), litOf(int64(1))))),
	}

	// Outer side's Degree field is not consumed (outer iteration covers
	// the whole LookupPop), but the proto requires the fixed count > 0.
	// Keep it at 1 as the documented convention.
	sides := []*dgproto.Side{
		{
			Population: childParentPop,
			Degree: &dgproto.Degree{Kind: &dgproto.Degree_Fixed{
				Fixed: &dgproto.DegreeFixed{Count: 1},
			}},
			Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{
				Sequential: &dgproto.StrategySequential{},
			}},
		},
		{
			Population: childIterPop,
			Degree: &dgproto.Degree{Kind: &dgproto.Degree_Fixed{
				Fixed: &dgproto.DegreeFixed{Count: childDegree},
			}},
			Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{
				Sequential: &dgproto.StrategySequential{},
			}},
		},
	}

	return &dgproto.InsertSpec{
		Table: childIterPop,
		Seed:  0xBADDCAFE,
		Source: &dgproto.RelSource{
			// Size must be > 0 per proto validation; the runtime derives
			// the real total from outerSize × innerDegree once the
			// relationship is installed.
			Population:  &dgproto.Population{Name: childIterPop, Size: childRowCount},
			Attrs:       attrs,
			ColumnOrder: childColumns,
			LookupPops:  []*dgproto.LookupPop{parents},
			Relationships: []*dgproto.Relationship{{
				Name:  childRelationship,
				Sides: sides,
			}},
			Iter: childRelationship,
		},
	}
}

// createChildrenTable (re)creates the target table. ResetSchema has
// already dropped the public schema, so this always runs against a
// fresh namespace.
func createChildrenTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	const ddl = `CREATE TABLE children (
		c_id int8 PRIMARY KEY,
		c_parent_id int8,
		c_line int8,
		c_label text
	)`
	if _, err := pool.Exec(context.Background(), ddl); err != nil {
		t.Fatalf("create children: %v", err)
	}
}

// copyChildren bulk-inserts rows into the children table via the
// Postgres COPY protocol and returns the insert count.
func copyChildren(t *testing.T, pool *pgxpool.Pool, rows [][]any) int64 {
	t.Helper()
	return copyRowsTo(t, pool, "children", childColumns, rows)
}

// TestRelationshipSmoke drives the Stage-C relationship runtime + Lookup
// evaluator end-to-end against tmpfs Postgres: build a 2-pop spec,
// iterate via NewRuntime + Next, bulk-load via CopyFrom, verify shape
// with SQL aggregates.
func TestRelationshipSmoke(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	createChildrenTable(t, pool)

	rt, err := runtime.NewRuntime(childSpec())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	rows := drainRuntime(t, rt)
	if int64(len(rows)) != childRowCount {
		t.Fatalf("runtime emitted %d rows, want %d", len(rows), childRowCount)
	}

	if got := copyChildren(t, pool, rows); got != childRowCount {
		t.Fatalf("CopyFrom inserted %d rows, want %d", got, childRowCount)
	}

	ctx := context.Background()

	if got := CountRows(t, pool, "children"); got != childRowCount {
		t.Fatalf("SELECT COUNT(*) = %d, want %d", got, childRowCount)
	}

	// c_id is unique and covers 1..30.
	var distinctIDs, minID, maxID int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT c_id), MIN(c_id), MAX(c_id) FROM children`,
	).Scan(&distinctIDs, &minID, &maxID); err != nil {
		t.Fatalf("id stats: %v", err)
	}
	if distinctIDs != childRowCount || minID != 1 || maxID != childRowCount {
		t.Fatalf("c_id: distinct=%d min=%d max=%d, want %d/1/%d",
			distinctIDs, minID, maxID, childRowCount, childRowCount)
	}

	// Each parent id (1..10) appears exactly `childDegree` times.
	parentRows, err := pool.Query(ctx,
		`SELECT c_parent_id, COUNT(*) FROM children GROUP BY c_parent_id ORDER BY c_parent_id`)
	if err != nil {
		t.Fatalf("parent distribution: %v", err)
	}
	var parentDist []struct {
		ID    int64
		Count int64
	}
	for parentRows.Next() {
		var id, count int64
		if err := parentRows.Scan(&id, &count); err != nil {
			parentRows.Close()
			t.Fatalf("scan parent distribution: %v", err)
		}
		parentDist = append(parentDist, struct {
			ID    int64
			Count int64
		}{id, count})
	}
	parentRows.Close()

	if int64(len(parentDist)) != childParentCount {
		t.Fatalf("distinct parent ids = %d, want %d", len(parentDist), childParentCount)
	}
	for i, entry := range parentDist {
		wantID := int64(i + 1)
		if entry.ID != wantID || entry.Count != childDegree {
			t.Fatalf("parent[%d] = (id=%d,count=%d), want (id=%d,count=%d)",
				i, entry.ID, entry.Count, wantID, childDegree)
		}
	}

	// c_line is 1..childDegree and each value appears childParentCount
	// times.
	lineRows, err := pool.Query(ctx,
		`SELECT c_line, COUNT(*) FROM children GROUP BY c_line ORDER BY c_line`)
	if err != nil {
		t.Fatalf("line distribution: %v", err)
	}
	var lineDist []struct {
		Line  int64
		Count int64
	}
	for lineRows.Next() {
		var line, count int64
		if err := lineRows.Scan(&line, &count); err != nil {
			lineRows.Close()
			t.Fatalf("scan line distribution: %v", err)
		}
		lineDist = append(lineDist, struct {
			Line  int64
			Count int64
		}{line, count})
	}
	lineRows.Close()

	if int64(len(lineDist)) != childDegree {
		t.Fatalf("distinct lines = %d, want %d", len(lineDist), childDegree)
	}
	for i, entry := range lineDist {
		wantLine := int64(i + 1)
		if entry.Line != wantLine || entry.Count != childParentCount {
			t.Fatalf("line[%d] = (line=%d,count=%d), want (line=%d,count=%d)",
				i, entry.Line, entry.Count, wantLine, childParentCount)
		}
	}

	// Spot-check every row matches the closed-form mapping implied by
	// deterministic ENTITY/LINE nesting:
	//   c_parent_id = floor((c_id-1)/childDegree) + 1
	//   c_line      = ((c_id-1) % childDegree)    + 1
	//   c_label     = fmt.Sprintf("P%03d-%d", c_parent_id, c_line)
	dbRows, err := pool.Query(ctx,
		`SELECT c_id, c_parent_id, c_line, c_label FROM children ORDER BY c_id`)
	if err != nil {
		t.Fatalf("fetch children: %v", err)
	}
	defer dbRows.Close()

	var idx int64 = 1
	for dbRows.Next() {
		var (
			cID, cParentID, cLine int64
			cLabel                string
		)
		if err := dbRows.Scan(&cID, &cParentID, &cLine, &cLabel); err != nil {
			t.Fatalf("scan child: %v", err)
		}
		if cID != idx {
			t.Fatalf("c_id at position %d = %d, want %d", idx, cID, idx)
		}
		wantParent := (idx-1)/childDegree + 1
		wantLine := (idx-1)%childDegree + 1
		wantLabel := fmt.Sprintf("P%03d-%d", wantParent, wantLine)
		if cParentID != wantParent {
			t.Fatalf("c_parent_id at c_id=%d = %d, want %d", cID, cParentID, wantParent)
		}
		if cLine != wantLine {
			t.Fatalf("c_line at c_id=%d = %d, want %d", cID, cLine, wantLine)
		}
		if cLabel != wantLabel {
			t.Fatalf("c_label at c_id=%d = %q, want %q", cID, cLabel, wantLabel)
		}
		idx++
	}
	if err := dbRows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	// One more explicit spot-check: c_id=7 lands at parent 3, line 1.
	var label7 string
	if err := pool.QueryRow(ctx,
		`SELECT c_label FROM children WHERE c_id = 7`).Scan(&label7); err != nil {
		t.Fatalf("label for c_id=7: %v", err)
	}
	if label7 != "P003-1" {
		t.Fatalf("label for c_id=7 = %q, want %q", label7, "P003-1")
	}
}

// TestRelationshipSmokeDeterminism rebuilds the spec twice and drains
// two independent Runtimes; the relationship path must emit byte-
// identical rows across runs (pure function of the spec).
func TestRelationshipSmokeDeterminism(t *testing.T) {
	rtA, err := runtime.NewRuntime(childSpec())
	if err != nil {
		t.Fatalf("NewRuntime A: %v", err)
	}
	rtB, err := runtime.NewRuntime(childSpec())
	if err != nil {
		t.Fatalf("NewRuntime B: %v", err)
	}

	rowsA := drainRuntime(t, rtA)
	rowsB := drainRuntime(t, rtB)

	if int64(len(rowsA)) != childRowCount {
		t.Fatalf("A emitted %d rows, want %d", len(rowsA), childRowCount)
	}
	if !reflect.DeepEqual(rowsA, rowsB) {
		t.Fatalf("two runtimes with the same spec produced divergent rows")
	}
}
