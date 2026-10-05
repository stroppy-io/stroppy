package dbgen

const (
	oLcntMin = 1
	oLcntMax = 7
	oSuppSd  = 10
	oClrkSd  = 11
	oOdateSd = 13
	oPrioSd  = 38
	oCkeySd  = 40
	oLcntSd  = 43
	oCmntLen = 49
	oCmntSd  = 12
	oClrkScl = 1000

	lQtySd        = 14
	lDcntSd       = 15
	lTaxSd        = 16
	lShipSd       = 17
	lSmodeSd      = 18
	lPkeySd       = 19
	lSkeySd       = 20
	lSdteSd       = 21
	lCdteSd       = 22
	lRdteSd       = 23
	lRflgSd       = 24
	lCmntLen      = 27
	lCmntSd       = 25
	pennies       = 100
	ordersPerCust = 10
)

var (
	ockeyMin dssHuge
	ockeyMax dssHuge
	odateMin dssHuge
	odateMax dssHuge
	ascDate  []string
)

type Order struct {
	OKey          dssHuge
	CustKey       dssHuge
	Status        string
	TotalPrice    dssHuge
	Date          string
	OrderPriority string
	Clerk         string
	ShipPriority  int64
	Comment       string
	Lines         []LineItem
}

func (g *Generator) sdOrder(child Table, skipCount dssHuge) {
	g.advanceStream(oLcntSd, skipCount, false)
	g.advanceStream(oCkeySd, skipCount, false)
	g.advanceStream(oCmntSd, skipCount*2, false)
	g.advanceStream(oSuppSd, skipCount, false)
	g.advanceStream(oClrkSd, skipCount, false)
	g.advanceStream(oPrioSd, skipCount, false)
	g.advanceStream(oOdateSd, skipCount, false)
}

func (g *Generator) makeOrder(idx dssHuge) *Order {
	delta := 1
	order := &Order{}
	order.OKey = makeSparse(idx)
	if scale >= 30000 {
		order.CustKey = g.random64(ockeyMin, ockeyMax, oCkeySd)
	} else {
		order.CustKey = g.random(ockeyMin, ockeyMax, oCkeySd)
	}

	// Comment: Orders are not present for all customers.
	// In fact, one-third of the customers do not have any order in the database.
	// The orders are assigned at random to two-thirds of the customers
	for order.CustKey%3 == 0 {
		order.CustKey += dssHuge(delta)
		order.CustKey = min(order.CustKey, ockeyMax)
		delta *= -1
	}
	tmpDate := g.random(odateMin, odateMax, oOdateSd)
	order.Date = ascDate[tmpDate-startDate]
	g.pickStr(&oPrioritySet, oPrioSd, &order.OrderPriority)
	order.Clerk = g.pickClerk()
	order.Comment = g.makeText(oCmntLen, oCmntSd)
	order.ShipPriority = 0
	order.TotalPrice = 0
	order.Status = "O"
	oCnt := 0
	lineCount := g.random(oLcntMin, oLcntMax, oLcntSd)

	for lCnt := dssHuge(0); lCnt < lineCount; lCnt++ {
		line := LineItem{}
		line.OKey = order.OKey
		line.LCnt = lCnt + 1
		line.Quantity = g.random(lQtyMin, lQtyMax, lQtySd)
		line.Discount = g.random(lDcntMin, lDcntMax, lDcntSd)
		line.Tax = g.random(lTaxMin, lTaxMax, lTaxSd)

		g.pickStr(&lInstructSet, lShipSd, &line.ShipInstruct)
		g.pickStr(&lSmodeSet, lSmodeSd, &line.ShipMode)
		line.Comment = g.makeText(lCmntLen, lCmntSd)

		if scale > 30000 {
			line.PartKey = g.random64(lPkeyMin, LPkeyMax, lPkeySd)
		} else {
			line.PartKey = g.random(lPkeyMin, LPkeyMax, lPkeySd)
		}

		rPrice := rpbRoutine(line.PartKey)
		suppNum := g.random(0, 3, lSkeySd)
		line.SuppKey = partSuppBridge(line.PartKey, suppNum)
		line.EPrice = rPrice * line.Quantity

		order.TotalPrice += ((line.EPrice * (100 - line.Discount)) / pennies) *
			(100 + line.Tax) / pennies

		sDate := g.random(lSdteMin, lSdteMax, lSdteSd)
		sDate += tmpDate

		cDate := g.random(lCdteMin, lCdteMax, lCdteSd)
		cDate += tmpDate

		rDate := g.random(lRdteMin, lRdteMax, lRdteSd)
		rDate += sDate
		line.SDate = ascDate[sDate-startDate]
		line.CDate = ascDate[cDate-startDate]
		line.RDate = ascDate[rDate-startDate]

		if julian(int(rDate)) <= currentDate {
			var tmpStr string
			g.pickStr(&lRflagSet, lRflgSd, &tmpStr)
			line.RFlag = tmpStr[0:1]
		} else {
			line.RFlag = "N"
		}

		if julian(int(sDate)) <= currentDate {
			oCnt++
			line.LStatus = "F"
		} else {
			line.LStatus = "O"
		}

		order.Lines = append(order.Lines, line)
	}
	if oCnt > 0 {
		order.Status = "P"
	}
	if oCnt == len(order.Lines) {
		order.Status = "F"
	}

	return order
}

func initOrder() {
	ockeyMin = 1
	ockeyMax = dssHuge(float64(tDefs[TCust].base) * scale)
	ascDate = makeAscDate()
	odateMin = startDate
	odateMax = startDate + totDate - (lSdteMax + lRdteMax) - 1
}
