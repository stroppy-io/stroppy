package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKitFormattingArtifacts(t *testing.T) {
	input := "NAME       VALUE\n---------- ----------\nAlice      %\nBob        [NULL]\nCara       -\n  3 record(s) selected.\nElapsed: 00:00:01.00\nSQL> Disconnected from Oracle\nWith database options\n"
	blocks, err := readBlocks([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks[0].Rows) != 3 {
		t.Fatalf("footer imported as data: %+v", blocks)
	}
	for _, row := range blocks[0].Rows {
		if row[1] != "" {
			t.Fatalf("NULL marker not normalized: %v", row)
		}
	}
}

func TestKitLatin1(t *testing.T) {
	blocks, err := readBlocks([]byte("COUNTRY\n--------------------\nC\xd4TE D'IVOIRE\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := blocks[0].Rows[0][0]; got != "CÔTE D'IVOIRE" {
		t.Fatalf("country = %q", got)
	}
}

func TestKitShiftedRuler(t *testing.T) {
	input := "C_LAST_NAME                    C_FIRST_NAME        S_STORE_NAME                                                                PAID\n" +
		"------------------------------ -------------------- -------------------------------------------------- ------------------------------\n" +
		"Martin                        Harold              bar                                                                      5834.88\n"
	blocks, err := readBlocks([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(blocks[0].Rows[0], "|"); got != "Martin|Harold|bar|5834.88" {
		t.Fatalf("shifted ruler cut text: %q", got)
	}
}

func TestKitQuery17Wrapped(t *testing.T) {
	widths := []int{16, 200, 2, 25, 23, 25, 23, 30, 57, 25, 27, 25, 27, 26}
	var ruler strings.Builder
	for i, width := range widths {
		if i != 0 {
			ruler.WriteByte(' ')
		}
		ruler.WriteString(strings.Repeat("-", width))
	}
	input := "I_ITEM_ID STORE_RETURNS_QUANTITYAV   STORE_RETURNS_QUANTITYSTDEV\n" + ruler.String() + "\n" +
		fmt.Sprintf("%-16s %-200s %-2s %25s %23s %25s %23s %30s %28s", "AAAAAAAAKPFEAAAA", "Recently right", "TN", "1", "99", "", "", "1", "66") + "\n" +
		strings.Repeat(" ", 83) + "1" + strings.Repeat(" ", 24) + "32\n"
	blocks, err := readWrappedQuery17([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	row := blocks[0].Rows[0]
	if len(row) != 15 || row[8] != "66" || row[11] != "1" || row[12] != "32" {
		t.Fatalf("wrapped values misplaced: %v", row)
	}
}

func TestKitAnswersRegenerate(t *testing.T) {
	directory := os.Getenv("TPCDS_ANSWERS_DIR")
	if directory == "" {
		t.Skip("TPCDS_ANSWERS_DIR not set")
	}
	answers, err := collectAnswers(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(answers) != 101 || len(answers["query_17"].Rows) != 1 || len(answers["query_34"].Rows) != 451 {
		t.Fatalf("unexpected kit answer structure: %d statements", len(answers))
	}
	if _, err := os.Stat(filepath.Join(directory, "96.ans")); err != nil {
		t.Fatal(err)
	}
	if got := answers["query_96"].Rows[0][0]; got != "870" {
		t.Fatalf("q96 answer = %s, want 870", got)
	}
}
