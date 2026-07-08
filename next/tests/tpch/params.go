package main

// TPC-H §2.4 pinned query parameter values. These are the substitution
// parameters the spec fixes for the 22 benchmark queries; they are not
// randomizable (the SF=1 reference answers are computed against exactly these
// values). Mirrors the queryParams map in v5 workloads/tpch/tx.ts.

import "github.com/stroppy-io/stroppy/next/driver"

// queryParam is one bound :parameter for a TPC-H query, applied to the prepared
// handle's reusable Args buffer by name. The values are the spec-pinned
// defaults; the workload runs each query once with these.
type queryParam struct {
	name string
	set  func(a *driver.Args)
}

// queryParams maps each query (q1..q22) to its §2.4 pinned parameters in the
// order the SQL references them. A query with no parameters (none here) would
// map to nil; every TPC-H query takes at least one.
var queryParams = map[string][]queryParam{
	"q1":  {{name: "delta", set: func(a *driver.Args) { a.SetInt64("delta", 90) }}},
	"q2":  {{name: "size", set: func(a *driver.Args) { a.SetInt64("size", 15) }}, {name: "type", set: func(a *driver.Args) { a.SetString("type", "BRASS") }}, {name: "region", set: func(a *driver.Args) { a.SetString("region", "EUROPE") }}},
	"q3":  {{name: "segment", set: func(a *driver.Args) { a.SetString("segment", "BUILDING") }}, {name: "date", set: func(a *driver.Args) { a.SetString("date", "1995-03-15") }}},
	"q4":  {{name: "date", set: func(a *driver.Args) { a.SetString("date", "1993-07-01") }}},
	"q5":  {{name: "region", set: func(a *driver.Args) { a.SetString("region", "ASIA") }}, {name: "date", set: func(a *driver.Args) { a.SetString("date", "1994-01-01") }}},
	"q6":  {{name: "date", set: func(a *driver.Args) { a.SetString("date", "1994-01-01") }}, {name: "discount", set: func(a *driver.Args) { a.SetFloat64("discount", 0.06) }}, {name: "quantity", set: func(a *driver.Args) { a.SetInt64("quantity", 24) }}},
	"q7":  {{name: "nation1", set: func(a *driver.Args) { a.SetString("nation1", "FRANCE") }}, {name: "nation2", set: func(a *driver.Args) { a.SetString("nation2", "GERMANY") }}},
	"q8":  {{name: "region", set: func(a *driver.Args) { a.SetString("region", "AMERICA") }}, {name: "nation", set: func(a *driver.Args) { a.SetString("nation", "BRAZIL") }}, {name: "type", set: func(a *driver.Args) { a.SetString("type", "ECONOMY ANODIZED STEEL") }}},
	"q9":  {{name: "color", set: func(a *driver.Args) { a.SetString("color", "green") }}},
	"q10": {{name: "date", set: func(a *driver.Args) { a.SetString("date", "1993-10-01") }}},
	// q11's fraction scales by 1/SF against the canonical dataset, matching v5's
	// 0.0001/SCALE_FACTOR: the threshold is a fraction of total nationwide
	// supply value, so it tracks the dataset size.
	"q11": {{name: "nation", set: func(a *driver.Args) { a.SetString("nation", "GERMANY") }}, {name: "fraction", set: func(a *driver.Args) { a.SetFloat64("fraction", 0.0001/scaleFactor) }}},
	"q12": {{name: "shipmode1", set: func(a *driver.Args) { a.SetString("shipmode1", "MAIL") }}, {name: "shipmode2", set: func(a *driver.Args) { a.SetString("shipmode2", "SHIP") }}, {name: "date", set: func(a *driver.Args) { a.SetString("date", "1994-01-01") }}},
	"q13": {{name: "word1", set: func(a *driver.Args) { a.SetString("word1", "special") }}, {name: "word2", set: func(a *driver.Args) { a.SetString("word2", "requests") }}},
	"q14": {{name: "date", set: func(a *driver.Args) { a.SetString("date", "1995-09-01") }}},
	"q15": {{name: "date", set: func(a *driver.Args) { a.SetString("date", "1996-01-01") }}},
	"q16": {
		{name: "brand", set: func(a *driver.Args) { a.SetString("brand", "Brand#45") }},
		{name: "type_prefix", set: func(a *driver.Args) { a.SetString("type_prefix", "MEDIUM POLISHED") }},
		{name: "s1", set: func(a *driver.Args) { a.SetInt64("s1", 49) }}, {name: "s2", set: func(a *driver.Args) { a.SetInt64("s2", 14) }},
		{name: "s3", set: func(a *driver.Args) { a.SetInt64("s3", 23) }}, {name: "s4", set: func(a *driver.Args) { a.SetInt64("s4", 45) }},
		{name: "s5", set: func(a *driver.Args) { a.SetInt64("s5", 19) }}, {name: "s6", set: func(a *driver.Args) { a.SetInt64("s6", 3) }},
		{name: "s7", set: func(a *driver.Args) { a.SetInt64("s7", 36) }}, {name: "s8", set: func(a *driver.Args) { a.SetInt64("s8", 9) }},
	},
	"q17": {{name: "brand", set: func(a *driver.Args) { a.SetString("brand", "Brand#23") }}, {name: "container", set: func(a *driver.Args) { a.SetString("container", "MED BOX") }}},
	"q18": {{name: "quantity", set: func(a *driver.Args) { a.SetInt64("quantity", 300) }}},
	"q19": {
		{name: "brand1", set: func(a *driver.Args) { a.SetString("brand1", "Brand#12") }}, {name: "brand2", set: func(a *driver.Args) { a.SetString("brand2", "Brand#23") }},
		{name: "brand3", set: func(a *driver.Args) { a.SetString("brand3", "Brand#34") }},
		{name: "q1", set: func(a *driver.Args) { a.SetInt64("q1", 1) }}, {name: "q2", set: func(a *driver.Args) { a.SetInt64("q2", 10) }}, {name: "q3", set: func(a *driver.Args) { a.SetInt64("q3", 20) }},
	},
	"q20": {{name: "color", set: func(a *driver.Args) { a.SetString("color", "forest") }}, {name: "nation", set: func(a *driver.Args) { a.SetString("nation", "CANADA") }}, {name: "date", set: func(a *driver.Args) { a.SetString("date", "1994-01-01") }}},
	"q21": {{name: "nation", set: func(a *driver.Args) { a.SetString("nation", "SAUDI ARABIA") }}},
	"q22": {
		{name: "cc1", set: func(a *driver.Args) { a.SetString("cc1", "13") }}, {name: "cc2", set: func(a *driver.Args) { a.SetString("cc2", "31") }},
		{name: "cc3", set: func(a *driver.Args) { a.SetString("cc3", "23") }}, {name: "cc4", set: func(a *driver.Args) { a.SetString("cc4", "29") }},
		{name: "cc5", set: func(a *driver.Args) { a.SetString("cc5", "30") }}, {name: "cc6", set: func(a *driver.Args) { a.SetString("cc6", "18") }},
		{name: "cc7", set: func(a *driver.Args) { a.SetString("cc7", "17") }},
	},
}

// queryNames is the canonical q1..q22 order the workload and the SF=1 validator
// both iterate in.
var queryNames = []string{
	"q1", "q2", "q3", "q4", "q5", "q6", "q7", "q8", "q9", "q10", "q11",
	"q12", "q13", "q14", "q15", "q16", "q17", "q18", "q19", "q20", "q21", "q22",
}
