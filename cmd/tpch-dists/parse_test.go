package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// sampleSingle — one block, mixed case keywords, `#` comments.
const sampleSingle = `
# comment before anything
BEGIN regions
count|3
AFRICA|1
AMERICA|1
ASIA|1
END regions
`

// sampleMulti — three adjacent blocks with banner comments, covering
// the "multiple distributions per file" dimension that this tool's
// uniform output preserves.
const sampleMulti = `
### banner one
begin category
COUNT|3
FURNITURE|1
STORAGE EQUIP|2
OTHER|5
end category

### banner two
BEGIN nations
COUNT|2
ARGENTINA|1
EGYPT|3
END nations

# signed weights (present in real dists.dss nations dist)
begin offsets
count|2
ALPHA|-4
BETA|2
end offsets
`

func TestParseStream_Single(t *testing.T) {
	dists, order, err := parseStream(strings.NewReader(sampleSingle))
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"regions"}) {
		t.Fatalf("order: %v", order)
	}
	d := dists["regions"]
	if d == nil {
		t.Fatalf("regions missing")
	}
	if !reflect.DeepEqual(d.Columns, []string{"value"}) {
		t.Errorf("columns: %v", d.Columns)
	}
	if !reflect.DeepEqual(d.WeightSets, []string{"default"}) {
		t.Errorf("weight_sets: %v", d.WeightSets)
	}
	if len(d.Rows) != 3 {
		t.Fatalf("rows: %d, want 3", len(d.Rows))
	}
	if !reflect.DeepEqual(d.Rows[1].Values, []string{"AMERICA"}) {
		t.Errorf("row[1].values: %v", d.Rows[1].Values)
	}
	if !reflect.DeepEqual(d.Rows[1].Weights, []int64{1}) {
		t.Errorf("row[1].weights: %v", d.Rows[1].Weights)
	}
}

func TestParseStream_MultipleBlocks(t *testing.T) {
	dists, order, err := parseStream(strings.NewReader(sampleMulti))
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	want := []string{"category", "nations", "offsets"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order: got %v, want %v", order, want)
	}
	// Each distribution must carry exactly one column and one weight-set
	// name — that's the uniform Dict shape this tool emits.
	for _, name := range want {
		d, ok := dists[name]
		if !ok {
			t.Fatalf("missing dist %q", name)
		}
		if len(d.Columns) != 1 || len(d.WeightSets) != 1 {
			t.Errorf("dist %q: columns=%v weight_sets=%v", name, d.Columns, d.WeightSets)
		}
	}
	// Weighted row.
	if dists["category"].Rows[2].Weights[0] != 5 {
		t.Errorf("category[2].weight = %d, want 5", dists["category"].Rows[2].Weights[0])
	}
	// Signed weights survive.
	if dists["offsets"].Rows[0].Weights[0] != -4 {
		t.Errorf("offsets[0].weight = %d, want -4", dists["offsets"].Rows[0].Weights[0])
	}
}

func TestParseStream_CommentsSkipped(t *testing.T) {
	input := `
#### header banner
# line one
# line two

BEGIN x
COUNT|1
# comment inside block
alpha|7
END x
# tail comment
`
	dists, _, err := parseStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	if len(dists) != 1 {
		t.Fatalf("want 1 dist, got %d", len(dists))
	}
	d := dists["x"]
	if len(d.Rows) != 1 || d.Rows[0].Values[0] != "alpha" {
		t.Errorf("row: %+v", d.Rows)
	}
}

func TestParseStream_Malformed(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "unterminated block",
			src: `BEGIN a
COUNT|1
x|1
`,
			want: "unterminated block",
		},
		{
			name: "end without begin",
			src:  "END a\n",
			want: "END with no matching BEGIN",
		},
		{
			name: "nested begin",
			src: `BEGIN a
COUNT|1
x|1
BEGIN b
`,
			want: `BEGIN "b" while "a" still open`,
		},
		{
			name: "count mismatch",
			src: `BEGIN a
COUNT|2
x|1
END a
`,
			want: "declared COUNT=2 but has 1 rows",
		},
		{
			name: "data outside block",
			src:  "alpha|1\n",
			want: "outside BEGIN/END",
		},
		{
			name: "bad weight",
			src: `BEGIN a
alpha|not-a-number
END a
`,
			want: `weight "not-a-number"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseStream(strings.NewReader(tc.src))
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
	dists, _, err := parseStream(strings.NewReader(sampleMulti))
	if err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	d := &doc{
		Version:       "1",
		Source:        "test",
		Distributions: dists,
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
