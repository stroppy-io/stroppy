package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// sample1 — minimal scalar distribution with a single weight column.
const sample1 = `
-- comment line
create calendar;
set types = (int, int, int, int);
set weights = 1;
add (1, 28, 2, 28: 1);
add (2, 29, 4, 56: 1);
`

// sample2 — joint distribution with explicit column/weight-set names and
// two weight profiles.
const sample2 = `
create returns_per_channel;
set types = (varchar, int);
set weights = 2;
set names = (channel, reason_sk : returns, sales);
add ("web", 1: 10, 50);
add ("store", 2: 20, 40);
-- trailing comment on its own line
add ("catalog", 3: 5, 30); -- inline comment
`

// sample3 — uniform dict: weights=0, `add` rows have values only.
const sample3 = `
create regions;
set types = (varchar);
set weights = 0;
add ("AFRICA");
add ("AMERICA");
`

func TestParseStream_Scalar(t *testing.T) {
	got, err := parseStream(strings.NewReader(sample1))
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if len(got) != 1 || got[0].name != "calendar" {
		t.Fatalf("want one dist `calendar`, got %+v", got)
	}
	d := got[0].toDict()
	wantCols := []string{"col1", "col2", "col3", "col4"}
	if !reflect.DeepEqual(d.Columns, wantCols) {
		t.Errorf("columns: got %v, want %v", d.Columns, wantCols)
	}
	if !reflect.DeepEqual(d.WeightSets, []string{"default"}) {
		t.Errorf("weight_sets: got %v, want [default]", d.WeightSets)
	}
	if len(d.Rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(d.Rows))
	}
	if !reflect.DeepEqual(d.Rows[1].Values, []string{"2", "29", "4", "56"}) {
		t.Errorf("row[1].values: got %v", d.Rows[1].Values)
	}
	if !reflect.DeepEqual(d.Rows[1].Weights, []int64{1}) {
		t.Errorf("row[1].weights: got %v, want [1]", d.Rows[1].Weights)
	}
}

func TestParseStream_MultiColumnMultiWeightSet(t *testing.T) {
	got, err := parseStream(strings.NewReader(sample2))
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want one dist, got %d", len(got))
	}
	d := got[0].toDict()
	if !reflect.DeepEqual(d.Columns, []string{"channel", "reason_sk"}) {
		t.Errorf("columns: %v", d.Columns)
	}
	if !reflect.DeepEqual(d.WeightSets, []string{"returns", "sales"}) {
		t.Errorf("weight_sets: %v", d.WeightSets)
	}
	if len(d.Rows) != 3 {
		t.Fatalf("rows: want 3, got %d", len(d.Rows))
	}
	if !reflect.DeepEqual(d.Rows[0].Values, []string{"web", "1"}) {
		t.Errorf("row[0].values: %v", d.Rows[0].Values)
	}
	if !reflect.DeepEqual(d.Rows[0].Weights, []int64{10, 50}) {
		t.Errorf("row[0].weights: %v", d.Rows[0].Weights)
	}
	if !reflect.DeepEqual(d.Rows[2].Values, []string{"catalog", "3"}) {
		t.Errorf("row[2].values: %v", d.Rows[2].Values)
	}
	if !reflect.DeepEqual(d.Rows[2].Weights, []int64{5, 30}) {
		t.Errorf("row[2].weights: %v", d.Rows[2].Weights)
	}
}

func TestParseStream_UniformDict(t *testing.T) {
	got, err := parseStream(strings.NewReader(sample3))
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	d := got[0].toDict()
	if len(d.WeightSets) != 0 {
		t.Errorf("uniform dict should have empty weight_sets, got %v", d.WeightSets)
	}
	for i, r := range d.Rows {
		if len(r.Weights) != 0 {
			t.Errorf("row[%d]: uniform dict should have empty weights, got %v", i, r.Weights)
		}
	}
}

func TestParseStream_CommentsSkipped(t *testing.T) {
	input := `
-- header comment
create tiny; -- inline after stmt
set types = (int); -- after
set weights = 1; -- after
add (1: 2); -- end
`
	got, err := parseStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if len(got) != 1 || got[0].name != "tiny" {
		t.Fatalf("bad parse: %+v", got)
	}
	if len(got[0].rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(got[0].rows))
	}
}

func TestParseStream_Malformed(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "add before create",
			src:  "add (1: 1);\n",
			want: "no active create",
		},
		{
			name: "weights count mismatch",
			src: `create x;
set types = (int);
set weights = 2;
add (1: 3);
`,
			want: "got 1 weights",
		},
		{
			name: "bad weights literal",
			src: `create x;
set types = (int);
set weights = 1;
add (1: notanumber);
`,
			want: `weight "notanumber"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseStream(strings.NewReader(tc.src))
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRoundTripJSON(t *testing.T) {
	nd, err := parseStream(strings.NewReader(sample2))
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	d := &doc{
		Version:       "1",
		Source:        "test",
		Distributions: map[string]*dict{nd[0].name: nd[0].toDict()},
	}
	blob, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back doc
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(d, &back) {
		t.Fatalf("round-trip mismatch:\n  orig: %+v\n  back: %+v", d, &back)
	}
}
