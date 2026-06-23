package dbgen

import (
	"fmt"
)

const (
	cPhneSd  = 28
	cAbalSd  = 29
	cMsegSd  = 30
	cAddrLen = 25
	cCmntLen = 73
	cAddrSd  = 26
	cCmntSd  = 31
	cAbalMin = -99999
	cAbalMax = 999999
	lNtrgSd  = 27
)

type Cust struct {
	CustKey    dssHuge
	Name       string
	Address    string
	NationCode dssHuge
	Phone      string
	Acctbal    dssHuge
	MktSegment string
	Comment    string
}

func (g *Generator) sdCust(child Table, skipCount dssHuge) {
	g.advanceStream(cAddrSd, skipCount*9, false)
	g.advanceStream(cCmntSd, skipCount*2, false)
	g.advanceStream(lNtrgSd, skipCount, false)
	g.advanceStream(cPhneSd, skipCount*3, false)
	g.advanceStream(cAbalSd, skipCount, false)
	g.advanceStream(cMsegSd, skipCount, false)
}

func (g *Generator) makeCust(idx dssHuge) *Cust {
	cust := &Cust{}
	cust.CustKey = idx
	cust.Name = fmt.Sprintf("Customer#%09d", idx)
	cust.Address = g.vStr(cAddrLen, cAddrSd)
	i := g.random(0, dssHuge(nations.count-1), lNtrgSd)
	cust.NationCode = i
	cust.Phone = g.genPhone(i, cPhneSd)
	cust.Acctbal = g.random(cAbalMin, cAbalMax, cAbalSd)
	g.pickStr(&cMsegSet, cMsegSd, &cust.MktSegment)
	cust.Comment = g.makeText(cCmntLen, cCmntSd)

	return cust
}
