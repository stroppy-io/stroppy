package dbgen

type PartSupp struct {
	PartKey dssHuge
	SuppKey dssHuge
	Qty     dssHuge
	SCost   dssHuge
	Comment string
}

func (g *Generator) sdPsupp(child Table, skipCount dssHuge) {
	for j := 0; j < suppPerPart; j++ {
		g.advanceStream(psQtySd, skipCount, false)
		g.advanceStream(psScstSd, skipCount, false)
		g.advanceStream(psCmntSd, skipCount*2, false)
	}
}

