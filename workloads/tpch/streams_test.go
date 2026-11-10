package tpch

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// QGEN 2.17.3, gregrahn/tpch-kit 852ad0a5ee31ebefeed884cea4188781dd9613a3:
// qgen -r 19620719 -p 1 -s 1 -l parameters.txt.
func TestStreamParamsMatchQGEN(t *testing.T) {
	want := map[string]map[string]any{
		"q1":  {"delta": 119},
		"q2":  {"size": 36, "type": "STEEL", "region": "AMERICA"},
		"q3":  {"segment": "BUILDING", "date": "1995-03-08"},
		"q4":  {"date": "1994-03-01"},
		"q5":  {"region": "EUROPE", "date": "1993-01-01"},
		"q6":  {"date": "1993-01-01", "discount": 0.08, "quantity": 25},
		"q7":  {"nation1": "VIETNAM", "nation2": "RUSSIA"},
		"q8":  {"nation": "RUSSIA", "region": "EUROPE", "type": "LARGE BRUSHED COPPER"},
		"q9":  {"color": "pale"},
		"q10": {"date": "1994-01-01"},
		"q11": {"nation": "PERU", "fraction": "0.0001000000"},
		"q12": {"shipmode1": "TRUCK", "shipmode2": "MAIL", "date": "1996-01-01"},
		"q13": {"word1": "unusual", "word2": "accounts"},
		"q14": {"date": "1996-08-01"},
		"q15": {"date": "1996-02-01"},
		"q16": {
			"brand": "Brand#35", "type_prefix": "ECONOMY PLATED",
			"s1": 38, "s2": 50, "s3": 22, "s4": 13, "s5": 5, "s6": 41, "s7": 28, "s8": 20,
		},
		"q17": {"brand": "Brand#54", "container": "JUMBO CAN"},
		"q18": {"quantity": 314},
		"q19": {"brand1": "Brand#45", "brand2": "Brand#32", "brand3": "Brand#14", "q1": 5, "q2": 13, "q3": 22},
		"q20": {"color": "yellow", "date": "1995-01-01", "nation": "ETHIOPIA"},
		"q21": {"nation": "IRAN"},
		"q22": {"cc1": "15", "cc2": "11", "cc3": "30", "cc4": "23", "cc5": "19", "cc6": "14", "cc7": "25"},
	}

	got, err := streamParams(1, 19620719)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range queryNames {
		if !reflect.DeepEqual(got[name], want[name]) {
			t.Errorf("%s parameters = %#v, want %#v", name, got[name], want[name])
		}
	}
}

func TestStreamOrdersAreComplete(t *testing.T) {
	var encoded []byte

	for stream, order := range streamOrders {
		seen := map[int]bool{}
		for _, query := range order {
			if query < 1 || query > 22 || seen[query] {
				t.Fatalf("stream %d has invalid query %d", stream, query)
			}

			seen[query] = true
			encoded = append(encoded, byte(query))
		}
	}

	if got := fmt.Sprintf("%x", sha256.Sum256(encoded)); got !=
		"6add5532979d8fa17eab27c72ec8a8cce1e352702b62af289e9fc0fa9c9e4de3" {
		t.Fatalf("Appendix A orderings changed: %s", got)
	}

	if streamOrders[0][0] != 14 || streamOrders[1][0] != 21 || streamOrders[2][0] != 6 || len(streamOrders) != 41 {
		t.Fatal("Appendix A stream identity changed")
	}
}

func TestStreamParamsAreReproducibleAndIndependent(t *testing.T) {
	first, err := streamParams(1, 42)
	if err != nil {
		t.Fatal(err)
	}

	again, err := streamParams(1, 42)
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatalf("seed did not reproduce parameters: %v", err)
	}

	other, err := streamParams(1, 43)
	if err != nil || reflect.DeepEqual(first, other) {
		t.Fatalf("distinct seed did not change parameters: %v", err)
	}

	for seed := int64(1); seed <= 200; seed++ {
		params, generateErr := streamParams(10, seed)
		if generateErr != nil {
			t.Fatal(generateErr)
		}

		if params["q7"]["nation1"] == params["q7"]["nation2"] || params["q12"]["shipmode1"] == params["q12"]["shipmode2"] {
			t.Fatal("distinct-value parameter constraint failed")
		}

		seen := map[any]bool{}

		for i := 1; i <= 8; i++ {
			value := params["q16"][fmt.Sprintf("s%d", i)]
			if seen[value] {
				t.Fatal("duplicate Q16 size")
			}

			seen[value] = true
		}

		if quantity := params["q18"]["quantity"].(int); quantity < 312 || quantity > 315 {
			t.Fatalf("Q18 quantity %d outside performance range", quantity)
		}
	}
}

func TestGeneratedDateBoundsUseStreamParams(t *testing.T) {
	w := &workload{isPicodata: true, needsEndDates: true}

	params, err := streamParams(1, 19620719)
	if err != nil {
		t.Fatal(err)
	}

	got := w.prepareParams(params)
	if got["q1"]["shipdate_cutoff"] != shiftDate("1998-12-01", -119, 0, 0) || got["q4"]["date_3m"] != "1994-06-01" {
		t.Fatalf("bounds not derived from stream parameters: %#v, %#v", got["q1"], got["q4"])
	}
}

func TestIncompleteStreamsHaveNoQueryRate(t *testing.T) {
	start := time.Now()

	w := &workload{streams: []queryStream{{
		ID: 1, Attempted: 22, Succeeded: 22, Complete: true, started: start, finished: start.Add(time.Second),
	}}}
	for _, test := range []struct {
		complete bool
		errors   int
		status   report.Status
		valid    bool
	}{
		{true, 0, report.StatusCompleted, true},
		{false, 0, report.StatusCompleted, false},
		{true, 1, report.StatusCompleted, false},
		{true, 0, report.StatusCompletedWithErrors, false},
		{true, 0, report.StatusFailed, false},
	} {
		w.streams[0].Complete = test.complete
		w.streams[0].Errors = test.errors

		contribution, err := w.streamContribution(bench.ReportContext{Status: test.status})
		if err != nil {
			t.Fatal(err)
		}

		value := contribution.Data.(streamReport)
		if (value.QueriesPerHour != nil) != test.valid || value.TPCCompliant ||
			(value.Status == "complete") != test.valid {
			t.Fatalf("unexpected stream rate/compliance: %#v", value)
		}
	}
}

func TestStreamDriversRequireRetainedSessions(t *testing.T) {
	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, driver := range []bench.DriverTypeName{
		bench.DriverPostgres, bench.DriverCRDB, bench.DriverYugabyte,
		bench.DriverMySQL, bench.DriverTiDB, bench.DriverPicodata, bench.DriverYDB, bench.DriverNoop,
	} {
		_, resolveErr := catalog.Resolve("tpch/tx",
			bench.ParamInputs{CLI: map[string]string{"streams": "2"}},
			map[string]bench.DriverConfig{"": {Kind: driver}})
		if (resolveErr == nil) != streamDriverSupported(driver) {
			t.Errorf("driver %s: unexpected stream support: %v", driver, resolveErr)
		}
	}
}

func TestStreamMeasurementSpansAllWorkers(t *testing.T) {
	start := time.Now()
	w := &workload{streams: []queryStream{
		{Complete: true, Succeeded: 22, started: start.Add(time.Second), finished: start.Add(2 * time.Second)},
		{Complete: true, Succeeded: 22, started: start, finished: start.Add(5 * time.Second)},
	}}

	contribution, err := w.streamContribution(bench.ReportContext{Status: report.StatusCompleted})
	if err != nil {
		t.Fatal(err)
	}

	value := contribution.Data.(streamReport)
	if value.MeasurementSeconds != 5 || value.QueriesPerHour == nil || *value.QueriesPerHour != 44*3600/5 {
		t.Fatalf("measurement did not span first submission through last result: %#v", value)
	}
}

func TestStreamsRejectEmptyQueryBodies(t *testing.T) {
	for _, body := range []string{"", " \n\t", "-- query intentionally absent\n"} {
		var sql strings.Builder
		for i, name := range queryNames {
			fmt.Fprintf(&sql, "--+ %s\n--= body\n", name)

			if i == 0 {
				sql.WriteString(body)
				sql.WriteByte('\n')
			} else {
				sql.WriteString("SELECT 1;\n")
			}
		}

		w := &workload{sql: bench.ParseSQL(sql.String()), driverType: bench.DriverPostgres, scaleFactor: 1}

		err := w.prepareStreams(streamSelection{enabled: true, count: 1, seed: 42})
		if !errors.Is(err, errStreamQuery) || !strings.Contains(err.Error(), "q1/body") {
			t.Fatalf("empty body %q: expected q1/body refusal, got %v", body, err)
		}

		if w.streams != nil {
			t.Fatalf("empty query generated streams: %#v", w.streams)
		}
	}
}

func TestStreamSelectionIsExplicit(t *testing.T) {
	catalog, err := bench.NewCatalog(Test)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		params map[string]string
		valid  bool
	}{
		{map[string]string{}, true},
		{map[string]string{"streams": "2"}, true},
		{map[string]string{"query-stream": "0"}, true},
		{map[string]string{"query-stream": "42"}, true},
		{map[string]string{"streams": "2", "vus": "1"}, false},
		{map[string]string{"streams": "2", "iterations": "1"}, false},
		{map[string]string{"streams": "2", "executor": "constant-vus"}, false},
		{map[string]string{"streams": "2", "query-stream": "0"}, false},
		{map[string]string{"query-seed": "2147483646", "streams": "2"}, false},
	} {
		_, resolveErr := catalog.Resolve("tpch/tx", bench.ParamInputs{CLI: test.params},
			map[string]bench.DriverConfig{"": {Kind: bench.DriverPostgres}})
		if (resolveErr == nil) != test.valid {
			t.Errorf("resolve %v: error %v, valid=%v", test.params, resolveErr, test.valid)
		}
	}
}
