package dsdgen

// Fan-out generation for the web sales channel: web_sales (parent) and
// web_returns (child). One newLineGen closure drives both tables, mirroring
// WebSalesRowGenerator + WebReturnsRowGenerator. The sales line draws on its own
// streamSet (sss); when the line is returned a paired returns row is built on a
// separate returns streamSet (srs), so emitting one table never perturbs the
// other's RNG state. Column layouts come from WebSalesGeneratorColumn.java and
// WebReturnsGeneratorColumn.java; output order/formatting from WebSalesRow.java
// and WebReturnsRow.java. Constants are prefixed wsl*/wrt* to avoid clashing with
// web_site.go's ws* identifiers.

// web_sales column stream layout (table-local indices), in WebSalesGeneratorColumn
// enum order so consumeRemaining keeps every per-row seed budget aligned.
const (
	wslSoldDateSk = iota
	wslSoldTimeSk
	wslShipDateSk
	wslItemSk
	wslBillCustomerSk
	wslBillCdemoSk
	wslBillHdemoSk
	wslBillAddrSk
	wslShipCustomerSk
	wslShipCdemoSk
	wslShipHdemoSk
	wslShipAddrSk
	wslWebPageSk
	wslWebSiteSk
	wslShipModeSk
	wslWarehouseSk
	wslPromoSk
	wslOrderNumber
	wslPricingQuantity
	wslPricingWholesaleCost
	wslPricingListPrice
	wslPricingSalesPrice
	wslPricingExtDiscountAmt
	wslPricingExtSalesPrice
	wslPricingExtWholesaleCost
	wslPricingExtListPrice
	wslPricingExtTax
	wslPricingCouponAmt
	wslPricingExtShipCost
	wslPricingNetPaid
	wslPricingNetPaidIncTax
	wslPricingNetPaidIncShip
	wslPricingNetPaidIncShipTax
	wslPricingNetProfit
	wslPricing
	wslNulls
	wslIsReturned
	wslPermutation
)

var webSalesCols = []GeneratorColumn{
	wslSoldDateSk:               {GlobalColumnNumber: 409, SeedsPerRow: 2},
	wslSoldTimeSk:               {GlobalColumnNumber: 410, SeedsPerRow: 2},
	wslShipDateSk:               {GlobalColumnNumber: 411, SeedsPerRow: 16},
	wslItemSk:                   {GlobalColumnNumber: 412, SeedsPerRow: 1},
	wslBillCustomerSk:           {GlobalColumnNumber: 413, SeedsPerRow: 1},
	wslBillCdemoSk:              {GlobalColumnNumber: 414, SeedsPerRow: 1},
	wslBillHdemoSk:              {GlobalColumnNumber: 415, SeedsPerRow: 1},
	wslBillAddrSk:               {GlobalColumnNumber: 416, SeedsPerRow: 1},
	wslShipCustomerSk:           {GlobalColumnNumber: 417, SeedsPerRow: 2},
	wslShipCdemoSk:              {GlobalColumnNumber: 418, SeedsPerRow: 2},
	wslShipHdemoSk:              {GlobalColumnNumber: 419, SeedsPerRow: 1},
	wslShipAddrSk:               {GlobalColumnNumber: 420, SeedsPerRow: 1},
	wslWebPageSk:                {GlobalColumnNumber: 421, SeedsPerRow: 16},
	wslWebSiteSk:                {GlobalColumnNumber: 422, SeedsPerRow: 16},
	wslShipModeSk:               {GlobalColumnNumber: 423, SeedsPerRow: 16},
	wslWarehouseSk:              {GlobalColumnNumber: 424, SeedsPerRow: 16},
	wslPromoSk:                  {GlobalColumnNumber: 425, SeedsPerRow: 16},
	wslOrderNumber:              {GlobalColumnNumber: 426, SeedsPerRow: 1},
	wslPricingQuantity:          {GlobalColumnNumber: 427, SeedsPerRow: 1},
	wslPricingWholesaleCost:     {GlobalColumnNumber: 428, SeedsPerRow: 1},
	wslPricingListPrice:         {GlobalColumnNumber: 429, SeedsPerRow: 0},
	wslPricingSalesPrice:        {GlobalColumnNumber: 430, SeedsPerRow: 0},
	wslPricingExtDiscountAmt:    {GlobalColumnNumber: 431, SeedsPerRow: 0},
	wslPricingExtSalesPrice:     {GlobalColumnNumber: 432, SeedsPerRow: 0},
	wslPricingExtWholesaleCost:  {GlobalColumnNumber: 433, SeedsPerRow: 0},
	wslPricingExtListPrice:      {GlobalColumnNumber: 434, SeedsPerRow: 0},
	wslPricingExtTax:            {GlobalColumnNumber: 435, SeedsPerRow: 0},
	wslPricingCouponAmt:         {GlobalColumnNumber: 436, SeedsPerRow: 0},
	wslPricingExtShipCost:       {GlobalColumnNumber: 437, SeedsPerRow: 0},
	wslPricingNetPaid:           {GlobalColumnNumber: 438, SeedsPerRow: 0},
	wslPricingNetPaidIncTax:     {GlobalColumnNumber: 439, SeedsPerRow: 0},
	wslPricingNetPaidIncShip:    {GlobalColumnNumber: 440, SeedsPerRow: 0},
	wslPricingNetPaidIncShipTax: {GlobalColumnNumber: 441, SeedsPerRow: 0},
	wslPricingNetProfit:         {GlobalColumnNumber: 442, SeedsPerRow: 0},
	wslPricing:                  {GlobalColumnNumber: 443, SeedsPerRow: 128},
	wslNulls:                    {GlobalColumnNumber: 444, SeedsPerRow: 32},
	wslIsReturned:               {GlobalColumnNumber: 445, SeedsPerRow: 16},
	wslPermutation:              {GlobalColumnNumber: 446, SeedsPerRow: 0},
}

// web_returns column stream layout (table-local indices), in
// WebReturnsGeneratorColumn enum order.
const (
	wrtReturnedDateSk = iota
	wrtReturnedTimeSk
	wrtItemSk
	wrtRefundedCustomerSk
	wrtRefundedCdemoSk
	wrtRefundedHdemoSk
	wrtRefundedAddrSk
	wrtReturningCustomerSk
	wrtReturningCdemoSk
	wrtReturningHdemoSk
	wrtReturningAddrSk
	wrtWebPageSk
	wrtReasonSk
	wrtOrderNumber
	wrtPricingQuantity
	wrtPricingNetPaid
	wrtPricingExtTax
	wrtPricingNetPaidIncTax
	wrtPricingFee
	wrtPricingExtShipCost
	wrtPricingRefundedCash
	wrtPricingReversedCharge
	wrtPricingStoreCredit
	wrtPricingNetLoss
	wrtPricing
	wrtNulls
)

var webReturnsCols = []GeneratorColumn{
	wrtReturnedDateSk:        {GlobalColumnNumber: 383, SeedsPerRow: 32},
	wrtReturnedTimeSk:        {GlobalColumnNumber: 384, SeedsPerRow: 32},
	wrtItemSk:                {GlobalColumnNumber: 385, SeedsPerRow: 16},
	wrtRefundedCustomerSk:    {GlobalColumnNumber: 386, SeedsPerRow: 16},
	wrtRefundedCdemoSk:       {GlobalColumnNumber: 387, SeedsPerRow: 16},
	wrtRefundedHdemoSk:       {GlobalColumnNumber: 388, SeedsPerRow: 16},
	wrtRefundedAddrSk:        {GlobalColumnNumber: 389, SeedsPerRow: 16},
	wrtReturningCustomerSk:   {GlobalColumnNumber: 390, SeedsPerRow: 16},
	wrtReturningCdemoSk:      {GlobalColumnNumber: 391, SeedsPerRow: 16},
	wrtReturningHdemoSk:      {GlobalColumnNumber: 392, SeedsPerRow: 16},
	wrtReturningAddrSk:       {GlobalColumnNumber: 393, SeedsPerRow: 16},
	wrtWebPageSk:             {GlobalColumnNumber: 394, SeedsPerRow: 16},
	wrtReasonSk:              {GlobalColumnNumber: 395, SeedsPerRow: 16},
	wrtOrderNumber:           {GlobalColumnNumber: 396, SeedsPerRow: 0},
	wrtPricingQuantity:       {GlobalColumnNumber: 397, SeedsPerRow: 0},
	wrtPricingNetPaid:        {GlobalColumnNumber: 398, SeedsPerRow: 0},
	wrtPricingExtTax:         {GlobalColumnNumber: 399, SeedsPerRow: 0},
	wrtPricingNetPaidIncTax:  {GlobalColumnNumber: 400, SeedsPerRow: 0},
	wrtPricingFee:            {GlobalColumnNumber: 401, SeedsPerRow: 0},
	wrtPricingExtShipCost:    {GlobalColumnNumber: 402, SeedsPerRow: 0},
	wrtPricingRefundedCash:   {GlobalColumnNumber: 403, SeedsPerRow: 0},
	wrtPricingReversedCharge: {GlobalColumnNumber: 404, SeedsPerRow: 0},
	wrtPricingStoreCredit:    {GlobalColumnNumber: 405, SeedsPerRow: 0},
	wrtPricingNetLoss:        {GlobalColumnNumber: 406, SeedsPerRow: 0},
	wrtPricing:               {GlobalColumnNumber: 407, SeedsPerRow: 80},
	wrtNulls:                 {GlobalColumnNumber: 408, SeedsPerRow: 32},
}

// web channel constants from WebSalesRowGenerator / Table.java.
const (
	webGiftPercentage = 7
	webReturnPercent  = 10

	// Null parameters (Table.WEB_SALES / Table.WEB_RETURNS).
	webSalesNullBasis       = 5
	webSalesNotNullBitMap   = 0x20008
	wslFirstColumnGlobal    = 409 // WS_SOLD_DATE_SK
	webReturnsNullBasis     = 900
	webReturnsNotNullBitMap = 0x2004
	wrtFirstColumnGlobal    = 383 // WR_RETURNED_DATE_SK
)

// webOrderInfo carries the order-level join keys shared by every line of one
// web order (and the columns the returns row reuses).
type webOrderInfo struct {
	soldDateSk     int64
	soldTimeSk     int64
	billCustomerSk int64
	billCdemoSk    int64
	billHdemoSk    int64
	billAddrSk     int64
	shipCustomerSk int64
	shipCdemoSk    int64
	shipHdemoSk    int64
	shipAddrSk     int64
	orderNumber    int64
}

// wslNullAt / wrtNullAt report whether the output column at the given global
// number is nulled by the row's bitmap (same bit offset as
// TableRowWithNulls.isNull, relative to each row's first column).
func wslNullAt(nullBitMap int64, globalCol int) bool {
	return nullBitMap&(int64(1)<<uint(globalCol-wslFirstColumnGlobal)) != 0
}

func wrtNullAt(nullBitMap int64, globalCol int) bool {
	return nullBitMap&(int64(1)<<uint(globalCol-wrtFirstColumnGlobal)) != 0
}

// webKeyOrNull renders a surrogate-key value, suppressed to nil when the column is
// nulled or the key is -1 (getStringOrNullForKey).
func webKeyOrNull(nullAt func(int64, int) bool, nb int64, globalCol int, v int64) any {
	if nullAt(nb, globalCol) || v == -1 {
		return nil
	}

	return v
}

// webValOrNull renders a non-key value, suppressed to nil when the column is nulled
// (getStringOrNull).
func webValOrNull(nullAt func(int64, int) bool, nb int64, globalCol int, v any) any {
	if nullAt(nb, globalCol) {
		return nil
	}

	return v
}

// webGenerateOrderInfo draws all order-level join keys for a new web order, in
// WebSalesRowGenerator.generateOrderInfo order on the sales streamSet.
func webGenerateOrderInfo(orderNumber int64, sss *streamSet, sc *Scaling) webOrderInfo {
	soldDateSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslSoldDateSk), TDateDim, 1, sc)
	soldTimeSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslSoldTimeSk), TTimeDim, 1, sc)
	billCustomerSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslBillCustomerSk), TCustomer, 1, sc)
	billCdemoSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslBillCdemoSk), TCustomerDemographics, 1, sc)
	billHdemoSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslBillHdemoSk), THouseholdDemographics, 1, sc)
	billAddrSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslBillAddrSk), TCustomerAddress, 1, sc)

	// Billing and shipping info match unless this is a gift.
	shipCustomerSk := billCustomerSk
	shipCdemoSk := billCdemoSk
	shipHdemoSk := billHdemoSk
	shipAddrSk := billAddrSk
	if GenerateUniformRandomInt(0, 99, sss.at(wslShipCustomerSk)) > webGiftPercentage {
		shipCustomerSk = GenerateJoinKey(TWebSales, JCNone, sss.at(wslShipCustomerSk), TCustomer, 2, sc)
		shipCdemoSk = GenerateJoinKey(TWebSales, JCNone, sss.at(wslShipCdemoSk), TCustomerDemographics, 2, sc)
		shipHdemoSk = GenerateJoinKey(TWebSales, JCNone, sss.at(wslShipHdemoSk), THouseholdDemographics, 2, sc)
		shipAddrSk = GenerateJoinKey(TWebSales, JCNone, sss.at(wslShipAddrSk), TCustomerAddress, 2, sc)
	}

	return webOrderInfo{
		soldDateSk: soldDateSk, soldTimeSk: soldTimeSk,
		billCustomerSk: billCustomerSk, billCdemoSk: billCdemoSk, billHdemoSk: billHdemoSk, billAddrSk: billAddrSk,
		shipCustomerSk: shipCustomerSk, shipCdemoSk: shipCdemoSk, shipHdemoSk: shipHdemoSk, shipAddrSk: shipAddrSk,
		orderNumber: orderNumber,
	}
}

// newWebLineGen builds the shared per-stream line generator for the web channel.
func newWebLineGen(sss, srs *streamSet, sc *Scaling) FactLineGen {
	itemCount := int(sc.IDCount(TItem))
	var itemPermutation []int
	var remainingLineItems int
	var order webOrderInfo
	var itemIndex int

	return func(ticket int64) ([]any, []any, bool) {
		if itemPermutation == nil {
			itemPermutation = MakePermutation(itemCount, sss.at(wslPermutation))
		}

		if remainingLineItems == 0 {
			order = webGenerateOrderInfo(ticket, sss, sc)
			itemIndex = GenerateUniformRandomInt(1, itemCount, sss.at(wslItemSk))
			remainingLineItems = GenerateUniformRandomInt(8, 16, sss.at(wslOrderNumber))
		}

		nullBitMap := CreateNullBitMap(webSalesNullBasis, webSalesNotNullBitMap, sss.at(wslNulls))

		shipLag := GenerateUniformRandomInt(1, 120, sss.at(wslShipDateSk))
		shipDateSk := order.soldDateSk + int64(shipLag)

		itemIndex++
		if itemIndex > itemCount {
			itemIndex = 1
		}
		itemSk := MatchSurrogateKey(int64(PermutationEntry(itemPermutation, itemIndex)), order.soldDateSk, TItem, sc)

		// the web page/site need to be valid for the sale date
		webPageSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslWebPageSk), TWebPage, order.soldDateSk, sc)
		webSiteSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslWebSiteSk), TWebSite, order.soldDateSk, sc)
		shipModeSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslShipModeSk), TShipMode, 1, sc)
		warehouseSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslWarehouseSk), TWarehouse, 1, sc)
		promoSk := GenerateJoinKey(TWebSales, JCNone, sss.at(wslPromoSk), TPromotion, 1, sc)
		pricing := GeneratePricingForSales(WebPricingLimits, sss.at(wslPricing))

		sales := buildWebSalesRow(nullBitMap, order, shipDateSk, itemSk, webPageSk, webSiteSk, shipModeSk, warehouseSk, promoSk, pricing)

		var returns []any
		if GenerateUniformRandomInt(0, 99, sss.at(wslIsReturned)) < webReturnPercent {
			returns = buildWebReturnsRow(srs, sc, order, itemSk, webPageSk, shipDateSk, pricing)
		}

		remainingLineItems--

		return sales, returns, remainingLineItems == 0
	}
}

// buildWebSalesRow renders one web_sales row in WebSalesRow.getValues order.
func buildWebSalesRow(nb int64, order webOrderInfo, shipDateSk, itemSk, webPageSk, webSiteSk, shipModeSk, warehouseSk, promoSk int64, p Pricing) []any {
	k := func(globalCol int, v int64) any { return webKeyOrNull(wslNullAt, nb, globalCol, v) }
	d := func(globalCol int, v Decimal) any { return webValOrNull(wslNullAt, nb, globalCol, v) }

	return []any{
		k(409, order.soldDateSk),
		k(410, order.soldTimeSk),
		k(411, shipDateSk),
		k(412, itemSk),
		k(413, order.billCustomerSk),
		k(414, order.billCdemoSk),
		k(415, order.billHdemoSk),
		k(416, order.billAddrSk),
		k(417, order.shipCustomerSk),
		k(418, order.shipCdemoSk),
		k(419, order.shipHdemoSk),
		k(420, order.shipAddrSk),
		k(421, webPageSk),
		k(422, webSiteSk),
		k(423, shipModeSk),
		k(424, warehouseSk),
		k(425, promoSk),
		k(426, order.orderNumber),
		webValOrNull(wslNullAt, nb, 427, int64(p.Quantity)),
		d(428, p.WholesaleCost),
		d(429, p.ListPrice),
		d(430, p.SalesPrice),
		d(431, p.ExtDiscountAmount),
		d(432, p.ExtSalesPrice),
		d(433, p.ExtWholesaleCost),
		d(434, p.ExtListPrice),
		d(435, p.ExtTax),
		d(436, p.CouponAmount),
		d(437, p.ExtShipCost),
		d(438, p.NetPaid),
		d(439, p.NetPaidIncludingTax),
		d(440, p.NetPaidIncludingShipping),
		d(441, p.NetPaidIncludingShippingAndTax),
		d(442, p.NetProfit),
	}
}

// buildWebReturnsRow ports WebReturnsRowGenerator.generateRow: it draws on the
// returns streamSet (srs) and reuses the sales row's keys/pricing.
func buildWebReturnsRow(srs *streamSet, sc *Scaling, order webOrderInfo, itemSk, webPageSk, shipDateSk int64, salesPricing Pricing) []any {
	nb := CreateNullBitMap(webReturnsNullBasis, webReturnsNotNullBitMap, srs.at(wrtNulls))

	returnedDateSk := GenerateJoinKey(TWebReturns, JCNone, srs.at(wrtReturnedDateSk), TDateDim, shipDateSk, sc)
	returnedTimeSk := GenerateJoinKey(TWebReturns, JCNone, srs.at(wrtReturnedTimeSk), TTimeDim, 1, sc)

	// items are usually returned to the people they were shipped to, but not always
	refundedCustomerSk := GenerateJoinKey(TWebReturns, JCNone, srs.at(wrtRefundedCustomerSk), TCustomer, 1, sc)
	refundedCdemoSk := GenerateJoinKey(TWebReturns, JCNone, srs.at(wrtRefundedCdemoSk), TCustomerDemographics, 1, sc)
	refundedHdemoSk := GenerateJoinKey(TWebReturns, JCNone, srs.at(wrtRefundedHdemoSk), THouseholdDemographics, 1, sc)
	refundedAddrSk := GenerateJoinKey(TWebReturns, JCNone, srs.at(wrtRefundedAddrSk), TCustomerAddress, 1, sc)
	if GenerateUniformRandomInt(0, 99, srs.at(wrtReturningCustomerSk)) < webGiftPercentage {
		refundedCustomerSk = order.shipCustomerSk
		refundedCdemoSk = order.shipCdemoSk
		refundedHdemoSk = order.shipHdemoSk
		refundedAddrSk = order.shipAddrSk
	}

	returningCustomerSk := refundedCustomerSk
	returningCdemoSk := refundedCdemoSk
	returningHdemoSk := refundedHdemoSk
	returningAddrSk := refundedAddrSk

	reasonSk := GenerateJoinKey(TWebReturns, JCNone, srs.at(wrtReasonSk), TReason, 1, sc)
	quantity := GenerateUniformRandomInt(1, salesPricing.Quantity, srs.at(wrtPricing))
	p := GeneratePricingForReturns(srs.at(wrtPricing), quantity, salesPricing)

	k := func(globalCol int, v int64) any { return webKeyOrNull(wrtNullAt, nb, globalCol, v) }
	d := func(globalCol int, v Decimal) any { return webValOrNull(wrtNullAt, nb, globalCol, v) }

	return []any{
		k(383, returnedDateSk),
		k(384, returnedTimeSk),
		k(385, itemSk),
		k(386, refundedCustomerSk),
		k(387, refundedCdemoSk),
		k(388, refundedHdemoSk),
		k(389, refundedAddrSk),
		k(390, returningCustomerSk),
		k(391, returningCdemoSk),
		k(392, returningHdemoSk),
		k(393, returningAddrSk),
		k(394, webPageSk),
		k(395, reasonSk),
		k(396, order.orderNumber),
		webValOrNull(wrtNullAt, nb, 397, int64(p.Quantity)),
		d(398, p.NetPaid),
		d(399, p.ExtTax),
		d(400, p.NetPaidIncludingTax),
		d(401, p.Fee),
		d(402, p.ExtShipCost),
		d(403, p.RefundedCash),
		d(404, p.ReversedCharge),
		d(405, p.StoreCredit),
		d(406, p.NetLoss),
	}
}

// WebSales is the TPC-DS web_sales fact table (the order/parent side).
var WebSales = &FactTable{
	Name: "web_sales",
	ID:   TWebSales,
	Columns: []string{
		"ws_sold_date_sk", "ws_sold_time_sk", "ws_ship_date_sk", "ws_item_sk",
		"ws_bill_customer_sk", "ws_bill_cdemo_sk", "ws_bill_hdemo_sk", "ws_bill_addr_sk",
		"ws_ship_customer_sk", "ws_ship_cdemo_sk", "ws_ship_hdemo_sk", "ws_ship_addr_sk",
		"ws_web_page_sk", "ws_web_site_sk", "ws_ship_mode_sk", "ws_warehouse_sk",
		"ws_promo_sk", "ws_order_number", "ws_quantity", "ws_wholesale_cost",
		"ws_list_price", "ws_sales_price", "ws_ext_discount_amt", "ws_ext_sales_price",
		"ws_ext_wholesale_cost", "ws_ext_list_price", "ws_ext_tax", "ws_coupon_amt",
		"ws_ext_ship_cost", "ws_net_paid", "ws_net_paid_inc_tax", "ws_net_paid_inc_ship",
		"ws_net_paid_inc_ship_tax", "ws_net_profit",
	},
	emitReturns: false,
	salesCols:   webSalesCols,
	returnsCols: webReturnsCols,
	TicketCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TWebSales) },
	newLineGen:  newWebLineGen,
}

// WebReturns is the TPC-DS web_returns fact table (the returns/child side); it
// shares web_sales' generation and emits only the returned lines.
var WebReturns = &FactTable{
	Name: "web_returns",
	ID:   TWebReturns,
	Columns: []string{
		"wr_returned_date_sk", "wr_returned_time_sk", "wr_item_sk", "wr_refunded_customer_sk",
		"wr_refunded_cdemo_sk", "wr_refunded_hdemo_sk", "wr_refunded_addr_sk",
		"wr_returning_customer_sk", "wr_returning_cdemo_sk", "wr_returning_hdemo_sk",
		"wr_returning_addr_sk", "wr_web_page_sk", "wr_reason_sk", "wr_order_number",
		"wr_return_quantity", "wr_return_amt", "wr_return_tax", "wr_return_amt_inc_tax",
		"wr_fee", "wr_return_ship_cost", "wr_refunded_cash", "wr_reversed_charge",
		"wr_account_credit", "wr_net_loss",
	},
	emitReturns: true,
	salesCols:   webSalesCols,
	returnsCols: webReturnsCols,
	TicketCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TWebSales) },
	newLineGen:  newWebLineGen,
}
