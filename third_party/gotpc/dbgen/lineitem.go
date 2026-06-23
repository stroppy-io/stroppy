package dbgen

const (
	lQtyMin  = 1
	lQtyMax  = 50
	lTaxMin  = 0
	lTaxMax  = 8
	lDcntMin = 0
	lDcntMax = 10
	lPkeyMin = 1
	lSdteMin = 1
	lSdteMax = 121
	lCdteMin = 30
	lCdteMax = 90
	lRdteMin = 1
	lRdteMax = 30
)

var (
	LPkeyMax dssHuge
)

type LineItem struct {
	OKey         dssHuge
	PartKey      dssHuge
	SuppKey      dssHuge
	LCnt         dssHuge
	Quantity     dssHuge
	EPrice       dssHuge
	Discount     dssHuge
	Tax          dssHuge
	RFlag        string
	LStatus      string
	CDate        string
	SDate        string
	RDate        string
	ShipInstruct string
	ShipMode     string
	Comment      string
}

func (g *Generator) sdLineItem(child Table, skipCount dssHuge) {
	for j := 0; j < oLcntMax; j++ {
		for i := lQtySd; i <= lRflgSd; i++ {
			g.advanceStream(i, skipCount, false)
		}
		g.advanceStream(lCmntSd, skipCount*2, false)
	}
	if child == TPsupp {
		g.advanceStream(oOdateSd, skipCount, false)
		g.advanceStream(oLcntSd, skipCount, false)
	}
}

func initLineItem() {
	LPkeyMax = dssHuge(float64(tDefs[TPart].base) * scale)
}
