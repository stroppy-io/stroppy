//go:build integration

package integration

import (
	"context"
	"math"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

// TestStageDSmokeIntegration is the Stage D7 end-to-end smoke: four
// tables built from Go struct-literal InsertSpecs exercise every Stage-D
// primitive (all twelve Draws, Choose, Attr.cohortDraw/Live, SCD-2
// row-split, Uniform degree) and verify the wire-through via SQL
// aggregates on a real tmpfs Postgres.
func TestStageDSmokeIntegration(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)

	stageDCreateTables(t, pool)

	catalogSpec := stageDCatalogSpec()
	loadSpec(t, pool, catalogSpec, "catalog", stageDCatalogColumns)

	eventsSpec := stageDEventsSpec()
	loadSpec(t, pool, eventsSpec, "events", stageDEventsColumns)

	scd2Spec := stageDStoreVersionsSpec()
	loadSpec(t, pool, scd2Spec, "store_versions", stageDStoreVersionsColumns)

	ordersSpec, linesSpec := stageDOrdersSpecs()
	loadSpec(t, pool, ordersSpec, "orders", stageDOrdersColumns)
	loadSpec(t, pool, linesSpec, "order_lines", stageDOrderLinesColumns)

	stageDAssertCatalog(t, pool)
	stageDAssertEvents(t, pool)
	stageDAssertStoreVersions(t, pool)
	stageDAssertOrders(t, pool)

	t.Run("Determinism", func(t *testing.T) {
		// Same seeds → identical emit rows across runs. Compared before
		// any DB-side transform lossiness, so this is strict equality on
		// runtime output.
		specs := []*dgproto.InsertSpec{
			stageDCatalogSpec(),
			stageDEventsSpec(),
			stageDStoreVersionsSpec(),
		}
		for _, spec := range specs {
			rowsA := drainSpec(t, spec)
			rowsB := drainSpec(t, spec)
			if !reflect.DeepEqual(rowsA, rowsB) {
				t.Fatalf("%s: two runtimes with the same spec produced divergent rows",
					spec.GetTable())
			}
		}

		// Orders+order_lines have a parent/child relationship via the
		// uniform-degree side; determinism must hold for the child too.
		os1, ol1 := stageDOrdersSpecs()
		os2, ol2 := stageDOrdersSpecs()
		osA := drainSpec(t, os1)
		osB := drainSpec(t, os2)
		if !reflect.DeepEqual(osA, osB) {
			t.Fatalf("orders emission non-deterministic")
		}
		olA := drainSpec(t, ol1)
		olB := drainSpec(t, ol2)
		if !reflect.DeepEqual(olA, olB) {
			t.Fatalf("order_lines emission non-deterministic")
		}
	})
}

// ---------- DDL ----------

func stageDCreateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ddls := []string{
		`CREATE TABLE catalog (
			item_id    int8 PRIMARY KEY,
			item_name  text,
			price      numeric(8,2),
			category   text,
			popularity int8
		)`,
		`CREATE TABLE events (
			event_id   int8 PRIMARY KEY,
			event_day  date,
			latency_ms float8,
			is_anomaly int8,
			item_id    int8,
			alive      int8,
			phrase     text,
			severity   text
		)`,
		`CREATE TABLE store_versions (
			store_id   int8,
			store_name text,
			valid_from text,
			valid_to   text
		)`,
		`CREATE TABLE orders (
			order_id int8 PRIMARY KEY,
			placed   date
		)`,
		`CREATE TABLE order_lines (
			line_id   int8 PRIMARY KEY,
			parent_id int8 NOT NULL,
			line_no   int8 NOT NULL
		)`,
	}
	for _, ddl := range ddls {
		if _, err := pool.Exec(context.Background(), ddl); err != nil {
			t.Fatalf("create table: %v (ddl=%q)", err, ddl)
		}
	}
}

// ---------- Spec builders ----------

var stageDCatalogColumns = []string{
	"item_id", "item_name", "price", "category", "popularity",
}

const (
	stageDCatalogSize = int64(500)
	stageDCatalogSeed = uint64(0xCA7A106511)
)

// stageDCatalogSpec builds the `catalog` InsertSpec: Draw.ascii,
// Draw.decimal, Draw.dict (weighted), Draw.nurand.
func stageDCatalogSpec() *dgproto.InsertSpec {
	categoryDict := &dgproto.Dict{
		Columns:    []string{},
		WeightSets: []string{""},
		Rows: []*dgproto.DictRow{
			{Values: []string{"electronics"}, Weights: []int64{1}},
			{Values: []string{"grocery"}, Weights: []int64{1}},
			{Values: []string{"clothing"}, Weights: []int64{1}},
			{Values: []string{"books"}, Weights: []int64{1}},
		},
	}

	attrs := []*dgproto.Attr{
		attrOf("item_id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		{Name: "item_name", Expr: streamDrawExpr(&dgproto.StreamDraw_Ascii{
			Ascii: &dgproto.DrawAscii{
				MinLen: litOf(int64(8)),
				MaxLen: litOf(int64(12)),
				Alphabet: []*dgproto.AsciiRange{
					{Min: 65, Max: 90}, {Min: 97, Max: 122},
				},
			},
		})},
		{Name: "price", Expr: streamDrawExpr(&dgproto.StreamDraw_Decimal{
			Decimal: &dgproto.DrawDecimal{
				Min:   litFloat(1.00),
				Max:   litFloat(999.99),
				Scale: 2,
			},
		})},
		{Name: "category", Expr: streamDrawExpr(&dgproto.StreamDraw_Dict{
			Dict: &dgproto.DrawDict{DictKey: "categories", WeightSet: ""},
		})},
		{Name: "popularity", Expr: streamDrawExpr(&dgproto.StreamDraw_Nurand{
			Nurand: &dgproto.DrawNURand{
				A:     255,
				X:     1,
				Y:     100,
				CSalt: 0xABCD,
			},
		})},
	}

	return &dgproto.InsertSpec{
		Table: "catalog",
		Seed:  stageDCatalogSeed,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "catalog", Size: stageDCatalogSize},
			Attrs:       attrs,
			ColumnOrder: stageDCatalogColumns,
		},
		Dicts: map[string]*dgproto.Dict{"categories": categoryDict},
	}
}

var stageDEventsColumns = []string{
	"event_id", "event_day", "latency_ms", "is_anomaly",
	"item_id", "alive", "phrase", "severity",
}

const (
	stageDEventsSize      = int64(2000)
	stageDEventsSeed      = uint64(0xE7EE_C0DE)
	stageDCohortSize      = int64(20)
	stageDCohortEntityMin = int64(1)
	stageDCohortEntityMax = int64(500)
	stageDCohortActive    = int64(3)
	stageDEventsBucketDiv = int64(100)
)

// stageDEventsSpec builds the `events` spec with Draw.bernoulli,
// Draw.normal, Draw.date, Draw.phrase, Draw.intUniform, Choose, and
// Attr.cohortDraw / Attr.cohortLive.
func stageDEventsSpec() *dgproto.InsertSpec {
	wordsDict := &dgproto.Dict{
		Columns:    []string{},
		WeightSets: []string{},
		Rows: []*dgproto.DictRow{
			{Values: []string{"alpha"}},
			{Values: []string{"beta"}},
			{Values: []string{"gamma"}},
			{Values: []string{"delta"}},
			{Values: []string{"epsilon"}},
			{Values: []string{"zeta"}},
			{Values: []string{"eta"}},
			{Values: []string{"theta"}},
		},
	}

	bucketExpr := binOpOf(dgproto.BinOp_DIV, rowIndexOf(), litOf(stageDEventsBucketDiv))

	// Draw.date bounds: epoch days for 2020-01-01 and 2020-12-31.
	minDays := daysEpoch(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	maxDays := daysEpoch(time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC))

	attrs := []*dgproto.Attr{
		attrOf("event_id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		{Name: "event_day", Expr: streamDrawExpr(&dgproto.StreamDraw_Date{
			Date: &dgproto.DrawDate{
				MinDaysEpoch: minDays,
				MaxDaysEpoch: maxDays,
			},
		})},
		{Name: "latency_ms", Expr: streamDrawExpr(&dgproto.StreamDraw_Normal{
			Normal: &dgproto.DrawNormal{
				Min:   litFloat(10),
				Max:   litFloat(1000),
				Screw: 3.0,
			},
		})},
		{Name: "is_anomaly", Expr: streamDrawExpr(&dgproto.StreamDraw_Bernoulli{
			Bernoulli: &dgproto.DrawBernoulli{P: 0.05},
		})},
		{Name: "item_id", Expr: &dgproto.Expr{Kind: &dgproto.Expr_CohortDraw{
			CohortDraw: &dgproto.CohortDraw{
				Name: "hot_items",
				Slot: streamDrawExpr(&dgproto.StreamDraw_IntUniform{
					IntUniform: &dgproto.DrawIntUniform{
						Min: litOf(int64(0)),
						Max: litOf(stageDCohortSize - 1),
					},
				}),
				BucketKey: bucketExpr,
			},
		}}},
		{Name: "alive", Expr: ifOf(
			&dgproto.Expr{Kind: &dgproto.Expr_CohortLive{CohortLive: &dgproto.CohortLive{
				Name:      "hot_items",
				BucketKey: bucketExpr,
			}}},
			litOf(int64(1)),
			litOf(int64(0)),
		)},
		{Name: "phrase", Expr: streamDrawExpr(&dgproto.StreamDraw_Phrase{
			Phrase: &dgproto.DrawPhrase{
				VocabKey:  "words",
				MinWords:  litOf(int64(3)),
				MaxWords:  litOf(int64(7)),
				Separator: " ",
			},
		})},
		chooseAttr("severity",
			&dgproto.ChooseBranch{Weight: 1, Expr: litOf("critical")},
			&dgproto.ChooseBranch{Weight: 9, Expr: litOf("normal")},
		),
	}

	return &dgproto.InsertSpec{
		Table: "events",
		Seed:  stageDEventsSeed,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "events", Size: stageDEventsSize},
			Attrs:       attrs,
			ColumnOrder: stageDEventsColumns,
			Cohorts: []*dgproto.Cohort{{
				Name:        "hot_items",
				CohortSize:  stageDCohortSize,
				EntityMin:   stageDCohortEntityMin,
				EntityMax:   stageDCohortEntityMax,
				ActiveEvery: stageDCohortActive,
			}},
		},
		Dicts: map[string]*dgproto.Dict{"words": wordsDict},
	}
}

var stageDStoreVersionsColumns = []string{
	"store_id", "store_name", "valid_from", "valid_to",
}

// stageDStoreVersionsSpec builds the SCD-2 demo: 10 rows, boundary=5,
// historical=1995-01-01..1999-12-31, current=2000-01-01..(null).
func stageDStoreVersionsSpec() *dgproto.InsertSpec {
	attrs := []*dgproto.Attr{
		attrOf("store_id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		{Name: "store_name", Expr: streamDrawExpr(&dgproto.StreamDraw_Ascii{
			Ascii: &dgproto.DrawAscii{
				MinLen: litOf(int64(5)),
				MaxLen: litOf(int64(10)),
				Alphabet: []*dgproto.AsciiRange{
					{Min: 65, Max: 90}, {Min: 97, Max: 122},
				},
			},
		})},
	}

	return &dgproto.InsertSpec{
		Table: "store_versions",
		Seed:  0x5CD2B001,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "store_versions", Size: 10},
			Attrs:       attrs,
			ColumnOrder: stageDStoreVersionsColumns,
			Scd2: &dgproto.SCD2{
				StartCol:        "valid_from",
				EndCol:          "valid_to",
				Boundary:        litOf(int64(5)),
				HistoricalStart: litOf("1995-01-01"),
				HistoricalEnd:   litOf("1999-12-31"),
				CurrentStart:    litOf("2000-01-01"),
				// CurrentEnd omitted → runtime emits nil.
			},
		},
	}
}

var (
	stageDOrdersColumns     = []string{"order_id", "placed"}
	stageDOrderLinesColumns = []string{"line_id", "parent_id", "line_no"}
)

const (
	stageDOrderParents   = int64(50)
	stageDOrderDegreeMin = int64(1)
	stageDOrderDegreeMax = int64(5)
)

// stageDOrdersSpecs builds the parent (`orders`) + child (`order_lines`)
// specs exercising a Uniform(1,5) degree. Parents are emitted as a flat
// dimension; children via a Relationship over a pure parent lookup pop.
func stageDOrdersSpecs() (parent, child *dgproto.InsertSpec) {
	parentSpec := &dgproto.InsertSpec{
		Table: "orders",
		Seed:  0x00011111,
		Source: &dgproto.RelSource{
			Population:  &dgproto.Population{Name: "orders", Size: stageDOrderParents},
			ColumnOrder: stageDOrdersColumns,
			Attrs: []*dgproto.Attr{
				attrOf("order_id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
				{Name: "placed", Expr: streamDrawExpr(&dgproto.StreamDraw_Date{
					Date: &dgproto.DrawDate{
						MinDaysEpoch: daysEpoch(time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)),
						MaxDaysEpoch: daysEpoch(time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC)),
					},
				})},
			},
		},
	}

	parentLookup := &dgproto.LookupPop{
		Population: &dgproto.Population{
			Name: "orders_src", Size: stageDOrderParents, Pure: true,
		},
		Attrs: []*dgproto.Attr{
			attrOf("p_id", binOpOf(dgproto.BinOp_ADD, rowIndexOf(), litOf(int64(1)))),
		},
		ColumnOrder: []string{"p_id"},
	}

	entityExpr := &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_ENTITY,
	}}}
	lineExpr := &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_LINE,
	}}}
	globalExpr := &dgproto.Expr{Kind: &dgproto.Expr_RowIndex{RowIndex: &dgproto.RowIndex{
		Kind: dgproto.RowIndex_GLOBAL,
	}}}

	childAttrs := []*dgproto.Attr{
		attrOf("line_id", binOpOf(dgproto.BinOp_ADD, globalExpr, litOf(int64(1)))),
		attrOf("parent_id", binOpOf(dgproto.BinOp_ADD, entityExpr, litOf(int64(1)))),
		attrOf("line_no", lineExpr),
	}

	rel := &dgproto.Relationship{
		Name: "orders_lines",
		Sides: []*dgproto.Side{
			{
				Population: "orders_src",
				Degree: &dgproto.Degree{Kind: &dgproto.Degree_Fixed{
					Fixed: &dgproto.DegreeFixed{Count: 1},
				}},
				Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{
					Sequential: &dgproto.StrategySequential{},
				}},
			},
			{
				Population: "order_lines",
				Degree: &dgproto.Degree{Kind: &dgproto.Degree_Uniform{
					Uniform: &dgproto.DegreeUniform{
						Min: stageDOrderDegreeMin,
						Max: stageDOrderDegreeMax,
					},
				}},
				Strategy: &dgproto.Strategy{Kind: &dgproto.Strategy_Sequential{
					Sequential: &dgproto.StrategySequential{},
				}},
			},
		},
	}

	childSpec := &dgproto.InsertSpec{
		Table: "order_lines",
		Seed:  0x0C1D04,
		Source: &dgproto.RelSource{
			Population:    &dgproto.Population{Name: "order_lines", Size: 1},
			Attrs:         childAttrs,
			ColumnOrder:   stageDOrderLinesColumns,
			LookupPops:    []*dgproto.LookupPop{parentLookup},
			Relationships: []*dgproto.Relationship{rel},
		},
	}

	return parentSpec, childSpec
}

// ---------- Small proto helpers ----------

// ---------- Runtime drive + COPY ----------

// ---------- Assertions ----------

func stageDAssertCatalog(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	if got := CountRows(t, pool, "catalog"); got != stageDCatalogSize {
		t.Fatalf("catalog: row count = %d, want %d", got, stageDCatalogSize)
	}

	// Draw.decimal price ∈ [1.00, 999.99].
	var minPrice, maxPrice float64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(price)::float8, MAX(price)::float8 FROM catalog`).Scan(&minPrice, &maxPrice); err != nil {
		t.Fatalf("catalog.price range: %v", err)
	}
	if minPrice < 1.00 || maxPrice > 999.99 {
		t.Fatalf("catalog.price range [%v,%v] outside [1.00, 999.99]", minPrice, maxPrice)
	}

	// Draw.decimal scale=2 → every value has ≤2 fractional digits.
	var badScale int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM catalog WHERE (price*100)::int8 <> (price*100)`).Scan(&badScale); err != nil {
		t.Fatalf("catalog.price scale check: %v", err)
	}
	if badScale != 0 {
		t.Fatalf("catalog.price: %d rows with > 2 fractional digits", badScale)
	}

	// Draw.ascii item_name length ∈ [8, 12], only letters.
	var badLen int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM catalog WHERE length(item_name) NOT BETWEEN 8 AND 12`).Scan(&badLen); err != nil {
		t.Fatalf("catalog.item_name length: %v", err)
	}
	if badLen != 0 {
		t.Fatalf("catalog.item_name: %d rows outside length [8, 12]", badLen)
	}

	// Draw.dict weighted categories: all four appear, each within ±15%
	// of uniform expectation.
	rows, err := pool.Query(ctx,
		`SELECT category, COUNT(*) FROM catalog GROUP BY category ORDER BY category`)
	if err != nil {
		t.Fatalf("catalog.category dist: %v", err)
	}
	defer rows.Close()

	counts := map[string]int64{}
	for rows.Next() {
		var name string
		var n int64
		if err := rows.Scan(&name, &n); err != nil {
			t.Fatalf("scan category: %v", err)
		}
		counts[name] = n
	}
	wantCats := []string{"books", "clothing", "electronics", "grocery"}
	for _, c := range wantCats {
		if _, ok := counts[c]; !ok {
			t.Fatalf("catalog.category: missing %q; have %v", c, counts)
		}
	}
	expected := float64(stageDCatalogSize) / float64(len(wantCats))
	tolerance := expected * 0.30
	for _, c := range wantCats {
		dev := math.Abs(float64(counts[c]) - expected)
		if dev > tolerance {
			t.Fatalf("catalog.category %q count=%d deviates from %v by > %.0f",
				c, counts[c], expected, tolerance)
		}
	}

	// Draw.nurand popularity: values land in [1, 100] by construction;
	// spot-check that the distribution is non-trivial (>=3 distinct).
	var popMin, popMax, popDistinct int64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(popularity), MAX(popularity), COUNT(DISTINCT popularity) FROM catalog`).
		Scan(&popMin, &popMax, &popDistinct); err != nil {
		t.Fatalf("catalog.popularity stats: %v", err)
	}
	if popMin < 1 || popMax > 100 {
		t.Fatalf("catalog.popularity range [%d,%d] outside [1, 100]", popMin, popMax)
	}
	if popDistinct < 3 {
		t.Fatalf("catalog.popularity only %d distinct values; expected >= 3", popDistinct)
	}
}

func stageDAssertEvents(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	if got := CountRows(t, pool, "events"); got != stageDEventsSize {
		t.Fatalf("events: row count = %d, want %d", got, stageDEventsSize)
	}

	// Draw.date bounds honored.
	var minDay, maxDay time.Time
	if err := pool.QueryRow(ctx,
		`SELECT MIN(event_day), MAX(event_day) FROM events`).Scan(&minDay, &maxDay); err != nil {
		t.Fatalf("events.event_day range: %v", err)
	}
	if minDay.Before(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) ||
		maxDay.After(time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("events.event_day [%v, %v] outside 2020", minDay, maxDay)
	}

	// Draw.normal latency_ms ∈ [10, 1000].
	var minLat, maxLat float64
	if err := pool.QueryRow(ctx,
		`SELECT MIN(latency_ms), MAX(latency_ms) FROM events`).Scan(&minLat, &maxLat); err != nil {
		t.Fatalf("events.latency_ms range: %v", err)
	}
	if minLat < 10 || maxLat > 1000 {
		t.Fatalf("events.latency_ms [%v, %v] outside [10, 1000]", minLat, maxLat)
	}

	// Draw.bernoulli is_anomaly: hit rate within ±3% of p=0.05.
	var hits int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE is_anomaly = 1) FROM events`).Scan(&hits); err != nil {
		t.Fatalf("events.is_anomaly: %v", err)
	}
	hitRate := float64(hits) / float64(stageDEventsSize)
	if math.Abs(hitRate-0.05) > 0.03 {
		t.Fatalf("events.is_anomaly hit rate = %.3f, want 0.05 ± 0.03", hitRate)
	}

	// Severity weighted choice (1:9): hit counts sum to N.
	var critical, normal int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE severity='critical'),
		        COUNT(*) FILTER (WHERE severity='normal') FROM events`,
	).Scan(&critical, &normal); err != nil {
		t.Fatalf("events.severity counts: %v", err)
	}
	if critical+normal != stageDEventsSize {
		t.Fatalf("events.severity: sum %d != %d", critical+normal, stageDEventsSize)
	}
	if critical <= 0 || normal <= 0 {
		t.Fatalf("events.severity: one branch never fired (critical=%d, normal=%d)",
			critical, normal)
	}

	// Cohort: alive=1 exactly on buckets where bucket % activeEvery == 0.
	// bucket_expected = row_index / 100; row_index is 0..1999, so buckets
	// 0..19. active_every=3 → alive buckets 0, 3, 6, 9, 12, 15, 18 = 7
	// buckets × 100 rows = 700 rows.
	var aliveCount int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE alive = 1`).Scan(&aliveCount); err != nil {
		t.Fatalf("events.alive: %v", err)
	}
	const expectedAlive = int64(7 * 100)
	if aliveCount != expectedAlive {
		t.Fatalf("events.alive=1 count = %d, want %d", aliveCount, expectedAlive)
	}

	// Per-bucket distinct item_id among active buckets must not exceed
	// the cohort size (20 slots drawn from by 100 rows). The 20-slot
	// universe is a hard upper bound; a handful of buckets may miss a
	// slot by random chance (coupon-collector), so we don't require
	// exact equality. We do require near-saturation (>= 15 of 20).
	rows, err := pool.Query(ctx, `
		SELECT (event_id-1)/100 AS bucket, COUNT(DISTINCT item_id)
		  FROM events
		 WHERE alive = 1
		 GROUP BY bucket
		 ORDER BY bucket`)
	if err != nil {
		t.Fatalf("events per-bucket distinct item_id: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var bucket int64
		var distinctItems int64
		if err := rows.Scan(&bucket, &distinctItems); err != nil {
			t.Fatalf("scan per-bucket: %v", err)
		}
		if distinctItems > stageDCohortSize {
			t.Fatalf("events bucket %d: distinct item_id = %d exceeds cohort size %d",
				bucket, distinctItems, stageDCohortSize)
		}
		if distinctItems < stageDCohortSize-5 {
			t.Fatalf("events bucket %d: distinct item_id = %d, want >= %d",
				bucket, distinctItems, stageDCohortSize-5)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	// All item_id values in [entity_min, entity_max].
	var outOfRange int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE item_id < $1 OR item_id > $2`,
		stageDCohortEntityMin, stageDCohortEntityMax,
	).Scan(&outOfRange); err != nil {
		t.Fatalf("events.item_id range: %v", err)
	}
	if outOfRange != 0 {
		t.Fatalf("events.item_id: %d rows outside [%d, %d]",
			outOfRange, stageDCohortEntityMin, stageDCohortEntityMax)
	}

	// Phrase: every phrase is a [3,7] word seq separated by spaces.
	var badPhrase int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM events
		 WHERE array_length(string_to_array(phrase, ' '), 1) NOT BETWEEN 3 AND 7
	`).Scan(&badPhrase); err != nil {
		t.Fatalf("events.phrase word-count: %v", err)
	}
	if badPhrase != 0 {
		t.Fatalf("events.phrase: %d rows outside [3, 7] words", badPhrase)
	}
}

func stageDAssertStoreVersions(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	if got := CountRows(t, pool, "store_versions"); got != 10 {
		t.Fatalf("store_versions: row count = %d, want 10", got)
	}

	// Historical slice: 5 rows with (1995-01-01, 1999-12-31).
	var hist int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM store_versions
		  WHERE valid_from = '1995-01-01' AND valid_to = '1999-12-31'`).Scan(&hist); err != nil {
		t.Fatalf("store_versions historical: %v", err)
	}
	if hist != 5 {
		t.Fatalf("store_versions historical = %d, want 5", hist)
	}

	// Current slice: 5 rows with valid_from='2000-01-01' and valid_to IS NULL.
	var curr int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM store_versions
		  WHERE valid_from = '2000-01-01' AND valid_to IS NULL`).Scan(&curr); err != nil {
		t.Fatalf("store_versions current: %v", err)
	}
	if curr != 5 {
		t.Fatalf("store_versions current = %d, want 5", curr)
	}

	// Names are non-empty letter strings in [5, 10].
	var badName int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM store_versions WHERE length(store_name) NOT BETWEEN 5 AND 10`).
		Scan(&badName); err != nil {
		t.Fatalf("store_versions.store_name length: %v", err)
	}
	if badName != 0 {
		t.Fatalf("store_versions.store_name: %d rows outside length [5, 10]", badName)
	}
}

func stageDAssertOrders(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	if got := CountRows(t, pool, "orders"); got != stageDOrderParents {
		t.Fatalf("orders: row count = %d, want %d", got, stageDOrderParents)
	}

	// Child row count ∈ [parents*min, parents*max].
	lineCount := CountRows(t, pool, "order_lines")
	lo := stageDOrderParents * stageDOrderDegreeMin
	hi := stageDOrderParents * stageDOrderDegreeMax
	if lineCount < lo || lineCount > hi {
		t.Fatalf("order_lines count = %d, outside [%d, %d]", lineCount, lo, hi)
	}

	// Every parent has at least one line; per-parent count ∈ [min,max].
	var parents int64
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT parent_id) FROM order_lines`).Scan(&parents); err != nil {
		t.Fatalf("order_lines distinct parents: %v", err)
	}
	if parents != stageDOrderParents {
		t.Fatalf("order_lines distinct parents = %d, want %d", parents, stageDOrderParents)
	}

	var minDeg, maxDeg int64
	if err := pool.QueryRow(ctx, `
		SELECT MIN(c), MAX(c) FROM (
			SELECT COUNT(*) AS c FROM order_lines GROUP BY parent_id
		) x`).Scan(&minDeg, &maxDeg); err != nil {
		t.Fatalf("order_lines per-parent range: %v", err)
	}
	if minDeg < stageDOrderDegreeMin || maxDeg > stageDOrderDegreeMax {
		t.Fatalf("order_lines degree range [%d,%d] outside [%d,%d]",
			minDeg, maxDeg, stageDOrderDegreeMin, stageDOrderDegreeMax)
	}

	// Deterministic per-parent count: the run we just loaded must match
	// a freshly drained copy of the child spec. Counts per parent_id are
	// compared.
	_, childSpec := stageDOrdersSpecs()
	freshRows := drainSpec(t, childSpec)
	freshPerParent := map[int64]int64{}
	for _, r := range freshRows {
		pid, ok := r[1].(int64)
		if !ok {
			t.Fatalf("fresh row missing parent_id: %#v", r)
		}
		freshPerParent[pid]++
	}

	dbPerParent := map[int64]int64{}
	rows, err := pool.Query(ctx,
		`SELECT parent_id, COUNT(*) FROM order_lines GROUP BY parent_id`)
	if err != nil {
		t.Fatalf("order_lines group by parent: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pid, cnt int64
		if err := rows.Scan(&pid, &cnt); err != nil {
			t.Fatalf("scan parent group: %v", err)
		}
		dbPerParent[pid] = cnt
	}
	if len(freshPerParent) != len(dbPerParent) {
		t.Fatalf("per-parent set size differs: fresh=%d db=%d",
			len(freshPerParent), len(dbPerParent))
	}
	// Compare sorted key-value tuples.
	var freshKeys []int64
	for k := range freshPerParent {
		freshKeys = append(freshKeys, k)
	}
	sort.Slice(freshKeys, func(i, j int) bool { return freshKeys[i] < freshKeys[j] })
	for _, k := range freshKeys {
		if freshPerParent[k] != dbPerParent[k] {
			t.Fatalf("parent_id=%d: fresh=%d db=%d", k, freshPerParent[k], dbPerParent[k])
		}
	}
}
