package main

import "testing"

func TestExpandTabs(t *testing.T) {
	// A tab advances to the next 8-column stop.
	got := expandTabs("ab\tc")
	want := "ab      c" // 2 + 6 spaces to col 8, then 'c'
	if got != want {
		t.Fatalf("expandTabs: got %q want %q", got, want)
	}
}

func TestFixedWidthSingleBlock(t *testing.T) {
	// Header is tab-packed (as the kit ships it); dashes and data use spaces.
	in := "C_ID\t C_NAME\n---------------- ----------\nAAAA1            Alice\nAAAA2            Bob Jones\n(2 rows)\n"
	blocks, err := readBlocks([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 {
		t.Fatalf("blocks: got %d want 1", len(blocks))
	}
	b := blocks[0]
	if len(b.Rows) != 2 {
		t.Fatalf("rows: got %d want 2", len(b.Rows))
	}
	// A value with an internal space must survive (no whitespace split).
	if b.Rows[1][1] != "Bob Jones" {
		t.Fatalf("row1 col1: got %q want %q", b.Rows[1][1], "Bob Jones")
	}
}

func TestPipeFormat(t *testing.T) {
	in := "A|B|C\n1|2|3\n4|5|6\n"
	blocks, err := readBlocks([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || len(blocks[0].Columns) != 3 || len(blocks[0].Rows) != 2 {
		t.Fatalf("pipe parse: %+v", blocks)
	}
	if blocks[0].Rows[1][2] != "6" {
		t.Fatalf("pipe cell: got %q", blocks[0].Rows[1][2])
	}
}

func TestTwoBlocks(t *testing.T) {
	// Two header/dashes/rows blocks back to back (two-part query shape).
	in := "X\n----\n1\n2\nY\n----\n9\n"
	blocks, err := readBlocks([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("blocks: got %d want 2", len(blocks))
	}
	if len(blocks[0].Rows) != 2 || len(blocks[1].Rows) != 1 {
		t.Fatalf("block rows: %d, %d", len(blocks[0].Rows), len(blocks[1].Rows))
	}
}

func TestMergePaginated(t *testing.T) {
	// A non-two-part query whose .ans reprinted the header mid-output: the two
	// blocks must merge into one query_N answer.
	in := "X\n----\n1\n2\nX\n----\n3\n"
	answers := map[string]*block{}
	blocks, err := readBlocks([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	mapBlocks(answers, 98, blocks, "98.ans")
	q := answers["query_98"]
	if q == nil || len(q.Rows) != 3 {
		t.Fatalf("merge: got %+v", q)
	}
}

func TestMapTwoPart(t *testing.T) {
	answers := map[string]*block{}
	in := "X\n----\n1\nY\n----\n2\n"
	blocks, _ := readBlocks([]byte(in))
	mapBlocks(answers, 14, blocks, "14.ans")
	if answers["query_14_a"] == nil || answers["query_14_b"] == nil {
		t.Fatalf("two-part keys missing: %v", sortedKeys(answers))
	}
}
