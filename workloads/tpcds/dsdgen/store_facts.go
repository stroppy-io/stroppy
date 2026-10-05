package dsdgen

// store_sales / store_returns: the fan-out fact channel for the store sales
// channel. One order ("ticket") fans out to 8..16 line items; each line emits a
// store_sales row and, 10% of the time, a paired store_returns row. The shared
// line generator below mirrors StoreSalesRowGenerator (sales + order info) and
// StoreReturnsRowGenerator.generateRow (the child return), reproducing dsdgen's
// per-stream RNG draw order exactly so both tables are byte-identical.

// Store sales column stream layout (table-local indices), in
// StoreSalesGeneratorColumn enum order. Global numbers and per-row seed counts
// are transcribed from that enum.
const (
	ssSoldDateSk = iota
	ssSoldTimeSk
	ssSoldItemSk
	ssSoldCustomerSk
	ssSoldCdemoSk
	ssSoldHdemoSk
	ssSoldAddrSk
	ssSoldStoreSk
	ssSoldPromoSk
	ssTicketNumber
	ssPricingQuantity
	ssPricingWholesaleCost
	ssPricingListPrice
	ssPricingSalesPrice
	ssPricingCouponAmt
	ssPricingExtSalesPrice
	ssPricingExtWholesaleCost
	ssPricingExtListPrice
	ssPricingExtTax
	ssPricingNetPaid
	ssPricingNetPaidIncTax
	ssPricingNetProfit
	srIsReturned
	ssPricing
	ssNulls
	ssPermutation
)

var storeSalesCols = []GeneratorColumn{
	ssSoldDateSk:              {GlobalColumnNumber: 314, SeedsPerRow: 2},
	ssSoldTimeSk:              {GlobalColumnNumber: 315, SeedsPerRow: 2},
	ssSoldItemSk:              {GlobalColumnNumber: 316, SeedsPerRow: 1},
	ssSoldCustomerSk:          {GlobalColumnNumber: 317, SeedsPerRow: 1},
	ssSoldCdemoSk:             {GlobalColumnNumber: 318, SeedsPerRow: 1},
	ssSoldHdemoSk:             {GlobalColumnNumber: 319, SeedsPerRow: 1},
	ssSoldAddrSk:              {GlobalColumnNumber: 320, SeedsPerRow: 1},
	ssSoldStoreSk:             {GlobalColumnNumber: 321, SeedsPerRow: 1},
	ssSoldPromoSk:             {GlobalColumnNumber: 322, SeedsPerRow: 16},
	ssTicketNumber:            {GlobalColumnNumber: 323, SeedsPerRow: 1},
	ssPricingQuantity:         {GlobalColumnNumber: 324, SeedsPerRow: 1},
	ssPricingWholesaleCost:    {GlobalColumnNumber: 325, SeedsPerRow: 0},
	ssPricingListPrice:        {GlobalColumnNumber: 326, SeedsPerRow: 0},
	ssPricingSalesPrice:       {GlobalColumnNumber: 327, SeedsPerRow: 0},
	ssPricingCouponAmt:        {GlobalColumnNumber: 328, SeedsPerRow: 0},
	ssPricingExtSalesPrice:    {GlobalColumnNumber: 329, SeedsPerRow: 0},
	ssPricingExtWholesaleCost: {GlobalColumnNumber: 330, SeedsPerRow: 0},
	ssPricingExtListPrice:     {GlobalColumnNumber: 331, SeedsPerRow: 0},
	ssPricingExtTax:           {GlobalColumnNumber: 332, SeedsPerRow: 0},
	ssPricingNetPaid:          {GlobalColumnNumber: 333, SeedsPerRow: 0},
	ssPricingNetPaidIncTax:    {GlobalColumnNumber: 334, SeedsPerRow: 0},
	ssPricingNetProfit:        {GlobalColumnNumber: 335, SeedsPerRow: 0},
	srIsReturned:              {GlobalColumnNumber: 336, SeedsPerRow: 16},
	ssPricing:                 {GlobalColumnNumber: 337, SeedsPerRow: 128},
	ssNulls:                   {GlobalColumnNumber: 338, SeedsPerRow: 32},
	ssPermutation:             {GlobalColumnNumber: 339, SeedsPerRow: 0},
}

// Store returns column stream layout, in StoreReturnsGeneratorColumn enum order.
const (
	srReturnedDateSk = iota
	srReturnedTimeSk
	srItemSk
	srCustomerSk
	srCdemoSk
	srHdemoSk
	srAddrSk
	srStoreSk
	srReasonSk
	srTicketNumber
	srPricingQuantity
	srPricingNetPaid
	srPricingExtTax
	srPricingNetPaidIncTax
	srPricingFee
	srPricingExtShipCost
	srPricingRefundedCash
	srPricingReversedCharge
	srPricingStoreCredit
	srPricingNetLoss
	srPricing
	srNulls
)

var storeReturnsCols = []GeneratorColumn{
	srReturnedDateSk:        {GlobalColumnNumber: 292, SeedsPerRow: 32},
	srReturnedTimeSk:        {GlobalColumnNumber: 293, SeedsPerRow: 32},
	srItemSk:                {GlobalColumnNumber: 294, SeedsPerRow: 16},
	srCustomerSk:            {GlobalColumnNumber: 295, SeedsPerRow: 16},
	srCdemoSk:               {GlobalColumnNumber: 296, SeedsPerRow: 16},
	srHdemoSk:               {GlobalColumnNumber: 297, SeedsPerRow: 16},
	srAddrSk:                {GlobalColumnNumber: 298, SeedsPerRow: 16},
	srStoreSk:               {GlobalColumnNumber: 299, SeedsPerRow: 16},
	srReasonSk:              {GlobalColumnNumber: 300, SeedsPerRow: 16},
	srTicketNumber:          {GlobalColumnNumber: 301, SeedsPerRow: 16},
	srPricingQuantity:       {GlobalColumnNumber: 302, SeedsPerRow: 0},
	srPricingNetPaid:        {GlobalColumnNumber: 303, SeedsPerRow: 0},
	srPricingExtTax:         {GlobalColumnNumber: 304, SeedsPerRow: 0},
	srPricingNetPaidIncTax:  {GlobalColumnNumber: 305, SeedsPerRow: 0},
	srPricingFee:            {GlobalColumnNumber: 306, SeedsPerRow: 0},
	srPricingExtShipCost:    {GlobalColumnNumber: 307, SeedsPerRow: 0},
	srPricingRefundedCash:   {GlobalColumnNumber: 308, SeedsPerRow: 0},
	srPricingReversedCharge: {GlobalColumnNumber: 309, SeedsPerRow: 0},
	srPricingStoreCredit:    {GlobalColumnNumber: 310, SeedsPerRow: 0},
	srPricingNetLoss:        {GlobalColumnNumber: 311, SeedsPerRow: 0},
	srPricing:               {GlobalColumnNumber: 312, SeedsPerRow: 80},
	srNulls:                 {GlobalColumnNumber: 313, SeedsPerRow: 32},
}

// Null parameters from Table.java: STORE_SALES nullBasisPoints 900, STORE_RETURNS
// nullBasisPoints 700; both share notNullBitMap 0x204.
const (
	storeSalesNullBasis    = 900
	storeReturnsNullBasis  = 700
	storeFactNotNullBitMap = 0x204

	ssFirstColumnGlobal = 314 // SS_SOLD_DATE_SK
	srFirstColumnGlobal = 292 // SR_RETURNED_DATE_SK

	storeReturnPct   = 10 // SR_RETURN_PCT
	storeSameCustPct = 80 // SR_SAME_CUSTOMER
)

// factIsNull reports whether the output column with the given global column
// number is nulled by the bitmap, using the same bit offset
// (globalColumnNumber - firstColumn) as TableRowWithNulls.isNull.
func factIsNull(nullBitMap int64, globalColumnNumber, firstColumnGlobal int) bool {
	off := globalColumnNumber - firstColumnGlobal

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// keyOrNull renders a surrogate/foreign key the way getStringOrNullForKey does:
// nil (empty field) when the column is nulled or the key is the -1 sentinel.
func keyOrNull(nullBitMap int64, globalColumnNumber, firstColumnGlobal int, value int64) any {
	if factIsNull(nullBitMap, globalColumnNumber, firstColumnGlobal) || value == -1 {
		return nil
	}

	return value
}

// valOrNull renders a non-key value the way getStringOrNull does: nil when the
// column is nulled, else the value itself.
func valOrNull(nullBitMap int64, globalColumnNumber, firstColumnGlobal int, value any) any {
	if factIsNull(nullBitMap, globalColumnNumber, firstColumnGlobal) {
		return nil
	}

	return value
}

// storeOrderInfo carries the order-level join keys drawn once per ticket, shared
// by every line item of the order. Mirrors StoreSalesRowGenerator.OrderInfo.
type storeOrderInfo struct {
	soldStoreSk    int64
	soldTimeSk     int64
	soldDateSk     int64
	soldCustomerSk int64
	soldCdemoSk    int64
	soldHdemoSk    int64
	soldAddrSk     int64
	ticketNumber   int64
}

// newStoreLineGen builds the shared per-stream line generator for the store
// channel. sss drives the sales/order draws, srs the returns draws; the two
// streamSets are independent so emitting one table never perturbs the other's
// bytes. Each call produces one line item of the given ticket.
func newStoreLineGen(sss, srs *streamSet, sc *Scaling) FactLineGen {
	var (
		itemPermutation    []int
		remainingLineItems int
		order              storeOrderInfo
		itemIndex          int
	)
	itemCount := int(sc.IDCount(TItem))

	return func(ticket int64) (sales []any, returns []any, endOrder bool) {
		// Item permutation is built lazily once; its column has 0 seeds/row so it
		// is partition-invariant.
		if itemPermutation == nil {
			itemPermutation = MakePermutation(itemCount, sss.at(ssPermutation))
		}

		if remainingLineItems == 0 {
			order = generateStoreOrderInfo(ticket, sss, sc)
			remainingLineItems = GenerateUniformRandomInt(8, 16, sss.at(ssTicketNumber))
			itemIndex = GenerateUniformRandomInt(1, itemCount, sss.at(ssSoldItemSk))
		}

		nullBitMap := CreateNullBitMap(storeSalesNullBasis, storeFactNotNullBitMap, sss.at(ssNulls))

		// items must be unique within an order: walk the permutation sequentially.
		itemIndex++
		if itemIndex > itemCount {
			itemIndex = 1
		}

		soldItemSk := MatchSurrogateKey(int64(PermutationEntry(itemPermutation, itemIndex)), order.soldDateSk, TItem, sc)
		soldPromoSk := GenerateJoinKey(TStoreSales, JCNone, sss.at(ssSoldPromoSk), TPromotion, 1, sc)
		pricing := GeneratePricingForSales(StorePricingLimits, sss.at(ssPricing))

		sales = buildStoreSalesRow(nullBitMap, &order, soldItemSk, soldPromoSk, pricing)

		// 10% of sales are returned. Always compute the return when it occurs (so
		// the store_returns table gets the row); drawing srs leaves the sales
		// bytes untouched since the streamSets are separate.
		isReturned := GenerateUniformRandomInt(0, 99, sss.at(srIsReturned)) < storeReturnPct
		if isReturned {
			returns = buildStoreReturnsRow(srs, sc, &order, soldItemSk, pricing)
		}

		remainingLineItems--
		endOrder = remainingLineItems == 0

		return sales, returns, endOrder
	}
}

// generateStoreOrderInfo draws the order-level join keys in the exact order of
// StoreSalesRowGenerator.generateOrderInfo.
func generateStoreOrderInfo(ticket int64, sss *streamSet, sc *Scaling) storeOrderInfo {
	return storeOrderInfo{
		soldStoreSk:    GenerateJoinKey(TStoreSales, JCNone, sss.at(ssSoldStoreSk), TStore, 1, sc),
		soldTimeSk:     GenerateJoinKey(TStoreSales, JCNone, sss.at(ssSoldTimeSk), TTimeDim, 1, sc),
		soldDateSk:     GenerateJoinKey(TStoreSales, JCNone, sss.at(ssSoldDateSk), TDateDim, 1, sc),
		soldCustomerSk: GenerateJoinKey(TStoreSales, JCNone, sss.at(ssSoldCustomerSk), TCustomer, 1, sc),
		soldCdemoSk:    GenerateJoinKey(TStoreSales, JCNone, sss.at(ssSoldCdemoSk), TCustomerDemographics, 1, sc),
		soldHdemoSk:    GenerateJoinKey(TStoreSales, JCNone, sss.at(ssSoldHdemoSk), THouseholdDemographics, 1, sc),
		soldAddrSk:     GenerateJoinKey(TStoreSales, JCNone, sss.at(ssSoldAddrSk), TCustomerAddress, 1, sc),
		ticketNumber:   ticket,
	}
}

// buildStoreSalesRow renders the store_sales row in StoreSalesRow.getValues order.
func buildStoreSalesRow(nb int64, order *storeOrderInfo, soldItemSk, soldPromoSk int64, p Pricing) []any {
	const f = ssFirstColumnGlobal

	return []any{
		keyOrNull(nb, 314, f, order.soldDateSk),
		keyOrNull(nb, 315, f, order.soldTimeSk),
		keyOrNull(nb, 316, f, soldItemSk),
		keyOrNull(nb, 317, f, order.soldCustomerSk),
		keyOrNull(nb, 318, f, order.soldCdemoSk),
		keyOrNull(nb, 319, f, order.soldHdemoSk),
		keyOrNull(nb, 320, f, order.soldAddrSk),
		keyOrNull(nb, 321, f, order.soldStoreSk),
		keyOrNull(nb, 322, f, soldPromoSk),
		keyOrNull(nb, 323, f, order.ticketNumber),
		valOrNull(nb, 324, f, int64(p.Quantity)),
		valOrNull(nb, 325, f, p.WholesaleCost),
		valOrNull(nb, 326, f, p.ListPrice),
		valOrNull(nb, 327, f, p.SalesPrice),
		valOrNull(nb, 328, f, p.CouponAmount),
		valOrNull(nb, 329, f, p.ExtSalesPrice),
		valOrNull(nb, 330, f, p.ExtWholesaleCost),
		valOrNull(nb, 331, f, p.ExtListPrice),
		valOrNull(nb, 332, f, p.ExtTax),
		valOrNull(nb, 328, f, p.CouponAmount),
		valOrNull(nb, 333, f, p.NetPaid),
		valOrNull(nb, 334, f, p.NetPaidIncludingTax),
		valOrNull(nb, 335, f, p.NetProfit),
	}
}

// buildStoreReturnsRow ports StoreReturnsRowGenerator.generateRow: it draws on
// the returns streamSet srs (in enum order) and reuses the sale's ticket, item,
// and pricing. The customer is the sale's 80% of the time.
func buildStoreReturnsRow(srs *streamSet, sc *Scaling, order *storeOrderInfo, soldItemSk int64, salesPricing Pricing) []any {
	nb := CreateNullBitMap(storeReturnsNullBasis, storeFactNotNullBitMap, srs.at(srNulls))

	ticketNumber := order.ticketNumber
	itemSk := soldItemSk

	customerSk := GenerateJoinKey(TStoreReturns, JCNone, srs.at(srCustomerSk), TCustomer, 1, sc)
	if GenerateUniformRandomInt(1, 100, srs.at(srTicketNumber)) < storeSameCustPct {
		customerSk = order.soldCustomerSk
	}

	returnedDateSk := GenerateJoinKey(TStoreReturns, JCNone, srs.at(srReturnedDateSk), TDateDim, order.soldDateSk, sc)
	returnedTimeSk := int64(GenerateUniformRandomInt(8*3600-1, 17*3600-1, srs.at(srReturnedTimeSk)))
	cdemoSk := GenerateJoinKey(TStoreReturns, JCNone, srs.at(srCdemoSk), TCustomerDemographics, 1, sc)
	hdemoSk := GenerateJoinKey(TStoreReturns, JCNone, srs.at(srHdemoSk), THouseholdDemographics, 1, sc)
	addrSk := GenerateJoinKey(TStoreReturns, JCNone, srs.at(srAddrSk), TCustomerAddress, 1, sc)
	storeSk := GenerateJoinKey(TStoreReturns, JCNone, srs.at(srStoreSk), TStore, 1, sc)
	reasonSk := GenerateJoinKey(TStoreReturns, JCNone, srs.at(srReasonSk), TReason, 1, sc)

	quantity := GenerateUniformRandomInt(1, salesPricing.Quantity, srs.at(srPricing))
	p := GeneratePricingForReturns(srs.at(srPricing), quantity, salesPricing)

	const f = srFirstColumnGlobal

	return []any{
		keyOrNull(nb, 292, f, returnedDateSk),
		keyOrNull(nb, 293, f, returnedTimeSk),
		keyOrNull(nb, 294, f, itemSk),
		keyOrNull(nb, 295, f, customerSk),
		keyOrNull(nb, 296, f, cdemoSk),
		keyOrNull(nb, 297, f, hdemoSk),
		keyOrNull(nb, 298, f, addrSk),
		keyOrNull(nb, 299, f, storeSk),
		keyOrNull(nb, 300, f, reasonSk),
		keyOrNull(nb, 301, f, ticketNumber),
		valOrNull(nb, 302, f, int64(p.Quantity)),
		valOrNull(nb, 303, f, p.NetPaid),
		valOrNull(nb, 304, f, p.ExtTax),
		valOrNull(nb, 305, f, p.NetPaidIncludingTax),
		valOrNull(nb, 306, f, p.Fee),
		valOrNull(nb, 307, f, p.ExtShipCost),
		valOrNull(nb, 308, f, p.RefundedCash),
		valOrNull(nb, 309, f, p.ReversedCharge),
		valOrNull(nb, 310, f, p.StoreCredit),
		valOrNull(nb, 311, f, p.NetLoss),
	}
}

// StoreSales is the store_sales fact table (the sales/parent side of the store
// channel). It emits one row per line item.
var StoreSales = &FactTable{
	Name: "store_sales",
	ID:   TStoreSales,
	Columns: []string{
		"ss_sold_date_sk", "ss_sold_time_sk", "ss_item_sk", "ss_customer_sk",
		"ss_cdemo_sk", "ss_hdemo_sk", "ss_addr_sk", "ss_store_sk", "ss_promo_sk",
		"ss_ticket_number", "ss_quantity", "ss_wholesale_cost", "ss_list_price",
		"ss_sales_price", "ss_ext_discount_amt", "ss_ext_sales_price",
		"ss_ext_wholesale_cost", "ss_ext_list_price", "ss_ext_tax", "ss_coupon_amt",
		"ss_net_paid", "ss_net_paid_inc_tax", "ss_net_profit",
	},
	emitReturns: false,
	salesCols:   storeSalesCols,
	returnsCols: storeReturnsCols,
	TicketCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TStoreSales) },
	newLineGen:  newStoreLineGen,
}

// StoreReturns is the store_returns fact table (the returns/child side). It emits
// only the lines that were returned, sharing StoreSales' generation.
var StoreReturns = &FactTable{
	Name: "store_returns",
	ID:   TStoreReturns,
	Columns: []string{
		"sr_returned_date_sk", "sr_return_time_sk", "sr_item_sk", "sr_customer_sk",
		"sr_cdemo_sk", "sr_hdemo_sk", "sr_addr_sk", "sr_store_sk", "sr_reason_sk",
		"sr_ticket_number", "sr_return_quantity", "sr_return_amt", "sr_return_tax",
		"sr_return_amt_inc_tax", "sr_fee", "sr_return_ship_cost", "sr_refunded_cash",
		"sr_reversed_charge", "sr_store_credit", "sr_net_loss",
	},
	emitReturns: true,
	salesCols:   storeSalesCols,
	returnsCols: storeReturnsCols,
	TicketCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TStoreSales) },
	newLineGen:  newStoreLineGen,
}
