package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestKitFormattingArtifacts(t *testing.T) {
	input := "NAME       VALUE\n---------- ----------\nAlice      %\nBob        [NULL]\nCara       -\n" +
		"  3 record(s) selected.\nElapsed: 00:00:01.00\nSQL> Disconnected from Oracle\nWith database options\n"

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

func TestMultipartPipeAnswers(t *testing.T) {
	blocks, err := readBlocks([]byte("A|B\n1|2\n3|4\n\nA|B\n3|4\n"))
	if err != nil {
		t.Fatal(err)
	}

	answers := map[string]*block{}
	mapBlocks(answers, 39, blocks, "39.ans")

	if len(answers["query_39_a"].Rows) != 2 || len(answers["query_39_b"].Rows) != 1 {
		t.Fatalf("multipart pipe answers not separated: %v", answers)
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
	input := fmt.Sprintf("%-30s %-19s %-50s %30s\n", "C_LAST_NAME", "C_FIRST_NAME", "S_STORE_NAME", "PAID") +
		strings.Repeat("-", 30) + " " + strings.Repeat("-", 20) + " " + strings.Repeat("-", 50) + " " +
		strings.Repeat("-", 30) + "\n" + fmt.Sprintf("%-29s %-19s %-50s %30s\n", "Martin", "Harold", "bar", "5834.88")

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
		fmt.Sprintf("%-16s %-200s %-2s %25s %23s %25s %23s %30s %28s",
			"AAAAAAAAKPFEAAAA", "Recently right", "TN", "1", "99", "", "", "1", "66") + "\n" +
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

	if len(answers) != 103 || len(answers["query_17"].Rows) != 1 || len(answers["query_34"].Rows) != 451 {
		t.Fatalf("unexpected kit answer structure: %d statements", len(answers))
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "workloads", "tpcds", "answers_sf1.json"))
	if err != nil {
		t.Fatal(err)
	}

	var committed doc
	if err := json.Unmarshal(data, &committed); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(answers, committed.Answers) {
		t.Fatal("committed answers differ from the supplied kit import")
	}

	if got := answers["query_96"].Rows[0][0]; got != "870" {
		t.Fatalf("q96 answer = %s, want 870", got)
	}
}
