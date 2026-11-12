package tpchgen

import (
	"errors"
	"math"

	"github.com/stroppy-io/stroppy/v6/workloads/tpch/dbgen"
)

var (
	errRefreshRange = errors.New("TPC-H refresh requires scale in [0.001,100000] and set in [1,4000]")
	errEmptyRefresh = errors.New("TPC-H refresh scale produces an empty DBGEN update set")
)

type RefreshGenerator struct {
	generator *dbgen.Generator
	set       int64
	next      int64
	count     int64
	remaining int64
}

func NewRefreshGenerator(scale float64, set int64) (*RefreshGenerator, error) {
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale < 0.001 || scale > 100000 || set < 1 || set > 4000 {
		return nil, errRefreshRange
	}

	generator := dbgen.NewGenerator(scale)
	base := int64(1500000 * scale)
	// DBGEN rounds the base cardinality before applying its 0.1% refresh percentage.
	count := base / 10000 * 10
	if count == 0 {
		return nil, errEmptyRefresh
	}

	generator.SeekOrderLine(base + (set-1)*count)

	return &RefreshGenerator{generator: generator, set: set, next: (set-1)*count + 1, count: count, remaining: count}, nil
}

func (g *RefreshGenerator) Count() int64 { return g.count }

func (g *RefreshGenerator) DeleteKey(index int64) int64 {
	return dbgen.SparseKey((g.set-1)*g.count+index+1, (g.set/1000)&3)
}

func (g *RefreshGenerator) Next() (*dbgen.Order, bool) {
	if g.remaining == 0 {
		return nil, false
	}

	g.generator.RowStart(dbgen.TOrder)
	order := g.generator.MakeRefreshOrder(g.next, g.set)
	g.generator.RowStop(dbgen.TOrder)

	g.next++
	g.remaining--

	return order, true
}
