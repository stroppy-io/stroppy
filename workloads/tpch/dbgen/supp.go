package dbgen

import (
	"fmt"
)

const (
	sNtrgSd      = 33
	sPhneSd      = 34
	sAbalSd      = 35
	sAddrLen     = 25
	sAbalMin     = -99999
	sAbalMax     = 999999
	sCmntLen     = 63
	sAddrSd      = 32
	sCmntSd      = 36
	sCmntBbb     = 10
	bbbJnkSd     = 44
	bbbTypeSd    = 45
	bbbCmntSd    = 46
	bbbOffsetSd  = 47
	bbbDeadbeats = 50
	bbbBase      = "Customer "
	bbbComplain  = "Complaints"
	bbbCommend   = "Recommends"
	bbbCmntLen   = 19
	bbbBaseLen   = 9
)

type Supp struct {
	SuppKey    dssHuge
	Name       string
	Address    string
	NationCode dssHuge
	Phone      string
	Acctbal    dssHuge
	Comment    string
}

func (g *Generator) makeSupp(idx dssHuge) *Supp {
	supp := &Supp{}
	supp.SuppKey = idx
	supp.Name = fmt.Sprintf("Supplier#%09d", idx)
	supp.Address = g.vStr(sAddrLen, sAddrSd)
	i := g.random(0, dssHuge(nations.count-1), sNtrgSd)
	supp.NationCode = i
	supp.Phone = g.genPhone(i, sPhneSd)
	supp.Acctbal = g.random(sAbalMin, sAbalMax, sAbalSd)
	supp.Comment = g.makeText(sCmntLen, sCmntSd)

	badPress := g.random(1, 10000, bbbCmntSd)
	types := g.random(0, 100, bbbTypeSd)
	noise := g.random(0, dssHuge(len(supp.Comment)-bbbCmntLen), bbbJnkSd)
	offset := g.random(0, dssHuge(len(supp.Comment))-(bbbCmntLen+noise), bbbOffsetSd)

	if badPress <= sCmntBbb {
		if types < bbbDeadbeats {
			types = 0
		} else {
			types = 1
		}
		supp.Comment = supp.Comment[:offset] + bbbBase + supp.Comment[offset+dssHuge(len(bbbBase)):]
		if types == 0 {
			supp.Comment = supp.Comment[:bbbBaseLen+offset+noise] +
				bbbComplain +
				supp.Comment[bbbBaseLen+offset+noise+dssHuge(len(bbbComplain)):]
		} else {
			supp.Comment = supp.Comment[:bbbBaseLen+offset+noise] +
				bbbCommend +
				supp.Comment[bbbBaseLen+offset+noise+dssHuge(len(bbbCommend)):]
		}
	}

	return supp
}

func (g *Generator) sdSupp(child Table, skipCount dssHuge) {
	g.advanceStream(sNtrgSd, skipCount, false)
	g.advanceStream(sPhneSd, skipCount*3, false)
	g.advanceStream(sAbalSd, skipCount, false)
	g.advanceStream(sAddrSd, skipCount*9, false)
	g.advanceStream(sCmntSd, skipCount*2, false)
	g.advanceStream(bbbCmntSd, skipCount, false)
	g.advanceStream(bbbJnkSd, skipCount, false)
	g.advanceStream(bbbOffsetSd, skipCount, false)
	g.advanceStream(bbbTypeSd, skipCount, false)
}
