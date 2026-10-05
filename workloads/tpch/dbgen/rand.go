package dbgen

import (
	"bytes"
	"fmt"
	"math"
)

const (
	maxStream         = 47
	rngA      dssHuge = 6364136223846793005
	rngC      dssHuge = 1
	maxLong           = math.MaxInt32
	dM                = 2147483647.0
	alphaNum          = "0123456789abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ,"
	vStrLow           = 0.4
	vStrHgh           = 1.6
)

var (
	multiplier = dssHuge(16807)
	modulus    = dssHuge(2147483647)
)

type Seed struct {
	Table
	value    dssHuge
	usage    dssHuge
	boundary dssHuge
}

func nextRand(nSeed dssHuge) dssHuge {
	return (nSeed * 16807) % 2147483647
}

func nextRand64(nSeed dssHuge) dssHuge {
	a := rngA
	c := rngC
	return nSeed*a + c
}

func (g *Generator) unifInt(nLow dssHuge, nHigh dssHuge, nStream long) dssHuge {
	var dRange float64
	var nTemp dssHuge
	nLow32 := int32(nLow)
	nHigh32 := int32(nHigh)

	if nStream < 0 || nStream > maxStream {
		nStream = 0
	}
	if (nHigh == maxLong) && (nLow == 0) {
		dRange = float64(nHigh32 - nLow32 + 1)
		_ = dssHuge(nHigh32 - nLow32 + 1)
	} else {
		dRange = float64(nHigh - nLow + 1)
		_ = nHigh - nLow + 1
	}
	g.seeds[nStream].value = nextRand(g.seeds[nStream].value)
	nTemp = dssHuge(float64(g.seeds[nStream].value) / dM * dRange)
	return nLow + nTemp
}

func (g *Generator) random64(lower, upper dssHuge, nStream long) dssHuge {

	if nStream < 0 || nStream > maxStream {
		nStream = 0
	}
	if lower > upper {
		lower, upper = upper, lower
	}
	g.seeds[nStream].value = nextRand64(g.seeds[nStream].value)

	nTemp := g.seeds[nStream].value
	if nTemp < 0 {
		nTemp = -nTemp
	}
	nTemp %= upper - lower + 1
	g.seeds[nStream].usage += 1
	return lower + nTemp
}

func (g *Generator) random(lower, upper dssHuge, nStream long) dssHuge {
	g.seeds[nStream].usage += 1
	return g.unifInt(lower, upper, nStream)
}

func advanceRand64(nSeed, nCount dssHuge) dssHuge {
	a := rngA
	c := rngC
	var nBit int
	aPow := a
	dSum := c
	if nCount == 0 {
		return nSeed
	}

	for nBit = 0; (nCount >> nBit) != rngC; nBit++ {
	}
	for {
		nBit -= 1
		if nBit < 0 {
			break
		}
		dSum *= aPow + 1
		aPow = aPow * aPow
		if (nCount>>nBit)%2 == 1 {
			dSum += aPow
			aPow *= a
		}
	}
	nSeed = nSeed*aPow + dSum*c
	return nSeed
}

func nthElement(n dssHuge, startSeed *dssHuge) {
	var z, mult dssHuge

	mult = multiplier
	z = *startSeed
	for n > 0 {
		if n%2 != 0 {
			z = (mult * z) % modulus
		}
		n = n / 2
		mult = (mult * mult) % modulus
	}
	*startSeed = z
}

func (g *Generator) advanceStream(nStream int, nCalls dssHuge, bUse64Bit bool) {
	if bUse64Bit {
		g.seeds[nStream].value = advanceRand64(g.seeds[nStream].value, nCalls)
	} else {
		nthElement(nCalls, &g.seeds[nStream].value)
	}
}

func (g *Generator) rowStart(_ Table) {
	for i := 0; i < maxStream; i++ {
		g.seeds[i].usage = 0
	}
}
func (g *Generator) rowStop(t Table) {
	if t == TOrderLine {
		t = TOrder
	}
	if t == TPartPsupp {
		t = TPart
	}

	for i := 0; i < maxStream; i++ {
		if g.seeds[i].Table == t || g.seeds[i].Table == tDefs[t].child {
			nthElement(g.seeds[i].boundary-g.seeds[i].usage, &g.seeds[i].value)
		}
	}
}

func (g *Generator) aRand(min, max, column int) string {
	var buf bytes.Buffer
	var charInt dssHuge
	len := g.random(dssHuge(min), dssHuge(max), long(column))
	for i := dssHuge(0); i < len; i++ {
		if i%5 == 0 {
			charInt = g.random(0, maxLong, long(column))
		}
		buf.Write([]byte{alphaNum[charInt&0o77]})
		charInt >>= 6
	}
	return buf.String()
}

func (g *Generator) vStr(avg, sd int) string {
	return g.aRand((int)(float64(avg)*vStrLow), (int)(float64(avg)*vStrHgh), sd)
}

func (g *Generator) genPhone(idx dssHuge, sd int) string {
	aCode := g.random(100, 999, long(sd))
	exChg := g.random(100, 999, long(sd))
	number := g.random(1000, 9999, long(sd))

	return fmt.Sprintf("%02d-%03d-%03d-%04d",
		10+(idx%nationsMax),
		aCode,
		exChg,
		number)
}

func (g *Generator) initSeeds() {
	g.seeds = [maxStream + 1]Seed{
		{TPart, 1, 0, 1},
		{TPart, 46831694, 0, 1},
		{TPart, 1841581359, 0, 1},
		{TPart, 1193163244, 0, 1},
		{TPart, 727633698, 0, 1},
		{TNone, 933588178, 0, 1},
		{TPart, 804159733, 0, 2},
		{TPsupp, 1671059989, 0, suppPerPart},
		{TPsupp, 1051288424, 0, suppPerPart},
		{TPsupp, 1961692154, 0, suppPerPart * 2},
		{TOrder, 1227283347, 0, 1},
		{TOrder, 1171034773, 0, 1},
		{TOrder, 276090261, 0, 2},
		{TOrder, 1066728069, 0, 1},
		{TLine, 209208115, 0, oLcntMax},
		{TLine, 554590007, 0, oLcntMax},
		{TLine, 721958466, 0, oLcntMax},
		{TLine, 1371272478, 0, oLcntMax},
		{TLine, 675466456, 0, oLcntMax},
		{TLine, 1808217256, 0, oLcntMax},
		{TLine, 2095021727, 0, oLcntMax},
		{TLine, 1769349045, 0, oLcntMax},
		{TLine, 904914315, 0, oLcntMax},
		{TLine, 373135028, 0, oLcntMax},
		{TLine, 717419739, 0, oLcntMax},
		{TLine, 1095462486, 0, oLcntMax * 2},
		{TCust, 881155353, 0, 9},
		{TCust, 1489529863, 0, 1},
		{TCust, 1521138112, 0, 3},
		{TCust, 298370230, 0, 1},
		{TCust, 1140279430, 0, 1},
		{TCust, 1335826707, 0, 2},
		{TSupp, 706178559, 0, 9},
		{TSupp, 110356601, 0, 1},
		{TSupp, 884434366, 0, 3},
		{TSupp, 962338209, 0, 1},
		{TSupp, 1341315363, 0, 2},
		{TPart, 709314158, 0, 92},
		{TOrder, 591449447, 0, 1},
		{TLine, 431918286, 0, 1},
		{TOrder, 851767375, 0, 1},
		{TNation, 606179079, 0, 2},
		{TRegion, 1500869201, 0, 2},
		{TOrder, 1434868289, 0, 1},
		{TSupp, 263032577, 0, 1},
		{TSupp, 753643799, 0, 1},
		{TSupp, 202794285, 0, 1},
		{TSupp, 715851524, 0, 1},
	}
}
