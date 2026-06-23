package dbgen

import (
	"fmt"
)

const (
	pNameScl    = 5
	pMfgMin     = 1
	pMfgMax     = 5
	pBrndMin    = 1
	pBrndMax    = 5
	pSizeMin    = 1
	pSizeMax    = 50
	psQtyMin    = 1
	psQtyMax    = 9999
	psScstMin   = 100
	psScstMax   = 100000
	pMfgSd      = 0
	pBrndSd     = 1
	pTypeSd     = 2
	pSizeSd     = 3
	pCntrSd     = 4
	psQtySd     = 7
	psScstSd    = 8
	pNameSd     = 37
	pCmntLen    = 14
	psCmntLen   = 124
	pCmntSd     = 6
	psCmntSd    = 9
	suppPerPart = 4
)

type Part struct {
	PartKey     dssHuge
	Name        string
	Mfgr        string
	Brand       string
	Type        string
	Size        dssHuge
	Container   string
	RetailPrice dssHuge
	Comment     string
	S           []PartSupp
}

func (g *Generator) sdPart(child Table, skipCount dssHuge) {
	for i := pMfgSd; i <= pCntrSd; i++ {
		g.advanceStream(i, skipCount, false)
	}
	g.advanceStream(pCmntSd, skipCount*2, false)
	g.advanceStream(pNameSd, skipCount*92, false)
}

func partSuppBridge(p, s dssHuge) dssHuge {
	totScnt := dssHuge(float64(tDefs[TSupp].base) * scale)
	return (p+s*(totScnt/suppPerPart+((p-1)/totScnt)))%totScnt + 1
}

func (g *Generator) makePart(idx dssHuge) *Part {
	part := &Part{}
	part.PartKey = idx
	part.Name = g.aggStr(&colors, pNameScl, pNameSd)
	tmp := g.random(pMfgMin, pMfgMax, pMfgSd)
	part.Mfgr = fmt.Sprintf("Manufacturer#%d", tmp)
	brnd := g.random(pBrndMin, pBrndMax, pBrndSd)
	part.Brand = fmt.Sprintf("Brand#%02d", tmp*10+brnd)
	g.pickStr(&pTypesSet, pTypeSd, &part.Type)
	part.Size = g.random(pSizeMin, pSizeMax, pSizeSd)
	g.pickStr(&pCntrSet, pCntrSd, &part.Container)
	part.RetailPrice = rpbRoutine(idx)
	part.Comment = g.makeText(pCmntLen, pCmntSd)

	for snum := 0; snum < suppPerPart; snum++ {
		ps := PartSupp{}
		ps.PartKey = part.PartKey
		ps.SuppKey = partSuppBridge(idx, dssHuge(snum))
		ps.Qty = g.random(psQtyMin, psQtyMax, psQtySd)
		ps.SCost = g.random(psScstMin, psScstMax, psScstSd)
		ps.Comment = g.makeText(psCmntLen, psCmntSd)
		part.S = append(part.S, ps)
	}

	return part
}
