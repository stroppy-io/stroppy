package main

import (
	"github.com/stroppy-io/stroppy/next/rng"
)

// TPC-C fixed cardinalities (spec §1.2 / §4.3.3).
const (
	districtsPerWarehouse = 10
	customersPerDistrict  = 3000
	itemsCount            = 100000
	stockPerWarehouse     = 100000
	ordersPerDistrict     = 3000
	// newOrdersPerDistrict is the initial new_order population per district: the
	// last 900 orders (o_id 2101..3000) are undelivered (§4.3.3.1).
	newOrdersPerDistrict = 900
	// deliveredThreshold splits delivered from undelivered orders: o_id < 2101 is
	// delivered at load (has a carrier, a delivery date, is not in new_order).
	deliveredThreshold = 2101
	// olQuantityLoad is the fixed order-line quantity at initial load (§4.3.3.1).
	olQuantityLoad = 5
)

// loadTimestamp is the fixed datetime text written into every load-time TIMESTAMP
// column. It is a constant (not time.Now) so a load is bit-reproducible across
// runs — the determinism contract requires identical data for identical
// (seed, WAREHOUSES). pgx encodes this string straight into a TIMESTAMP column
// through the binary COPY path (verified against pg16).
const loadTimestamp = "2024-01-01 00:00:00"

// world is the read-only, run-global generation context shared by every load
// generator and by the workload transactions. It is built once from the run seed
// in newWorld and never mutated, so it is safe to share by pointer across VUs.
type world struct {
	// warehouses is the configured warehouse count (W).
	warehouses int64

	// olCnt yields o_ol_cnt for an order. It is a run-global stream (not a
	// per-step one) because both the orders generator and the order_line
	// generator must agree on an order's line count, and they run under
	// different step ids — a per-step stream would disagree and break the
	// sum(o_ol_cnt) == count(order_line) consistency condition.
	olCnt rng.Stream

	// c_last / c_id / ol_i_id NURand run constants (§2.1.6). cLastLoad is the
	// load-time C(c_last); cLastRun is the run-time C(c_last), which must differ
	// from cLastLoad by a valid delta (§2.1.6.1) so the run's hot-name set is not
	// identical to the loaded one. cID and olID are run-only, so a single
	// constant each suffices.
	cLastLoad int64
	cLastRun  int64
	cID       int64
	olID      int64
}

// constStepID seeds the run-global constant streams. It is distinct from every
// real step's FNV-32a id (which derives from a non-empty step name) so the
// constant streams never collide with a step's data streams.
const constStepID = 0xC0FFEE

// newWorld builds the run-global generation context from the run seed.
func newWorld(seed uint64, warehouses int64) *world {
	w := &world{warehouses: warehouses}
	w.olCnt = rng.Derive(seed, constStepID, 1)

	// c_last: load constant in [0,255], run constant = load + valid delta.
	w.cLastLoad = rng.NURandConst(rng.Derive(seed, constStepID, 2), 255)
	w.cLastRun = w.cLastLoad + validDelta(rng.Derive(seed, constStepID, 3))

	w.cID = rng.NURandConst(rng.Derive(seed, constStepID, 4), 1023)
	w.olID = rng.NURandConst(rng.Derive(seed, constStepID, 5), 8191)
	return w
}

// validDelta draws a valid C(c_last) load/run delta per §2.1.6.1: an integer in
// [65,119] excluding 96 and 112. The 53 valid values are enumerated and indexed
// by one uniform draw, so the result is a pure function of the stream.
func validDelta(s rng.Stream) int64 {
	// Build the valid set once per call (53 values); cheap and plan-phase only.
	valid := make([]int64, 0, 53)
	for v := int64(65); v <= 119; v++ {
		if v == 96 || v == 112 {
			continue
		}
		valid = append(valid, v)
	}
	return valid[rng.UniformInt(s, 0, 0, int64(len(valid)-1))]
}

// orderOlCnt returns o_ol_cnt for the order at global order cycle oc: a uniform
// integer in [5,15] (§4.3.3.1). Pure in (world seed, oc); used by both the
// orders and order_line generators.
func (w *world) orderOlCnt(oc int64) int64 {
	return rng.UniformInt(w.olCnt, uint64(oc), 5, 15)
}

// splitMix64 is a local copy of the SplitMix64 finalizer, used by the o_c_id
// permutation. (rng's copy is unexported; this is the same Steele/Lea/Flood 2014
// mixer.)
func splitMix64(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

// permuteOCID maps an order's o_id (1..3000) to its o_c_id (1..3000) so that
// o_c_id is a permutation of the customer numbers within a district (§4.3.3.1:
// each customer places exactly one order). It is a stateless bijection over
// [0,3000): a 4-round Feistel network with a SplitMix64 round function, keyed by
// (w_id, d_id), applied with cycle-walking because 3000 is not a perfect square.
// Pure and allocation-free.
func permuteOCID(wID, dID, oID int64) int64 {
	key := splitMix64(uint64(wID)*1_000_003 + uint64(dID)*7 + 0x1BEEF)
	x := uint64(oID - 1)
	for {
		x = feistel(x, key)
		if x < customersPerDistrict {
			break
		}
	}
	return int64(x) + 1
}

// feistelDomain is the smallest even power split covering [0,3000): 64*64 = 4096.
const feistelHalf = 64

// feistel applies a 4-round Feistel permutation over [0, feistelHalf^2).
func feistel(x, key uint64) uint64 {
	l := x / feistelHalf
	r := x % feistelHalf
	for round := uint64(0); round < 4; round++ {
		f := splitMix64(r+key+round*0x9E3779B1) % feistelHalf
		l, r = r, (l+f)%feistelHalf
	}
	return l*feistelHalf + r
}
