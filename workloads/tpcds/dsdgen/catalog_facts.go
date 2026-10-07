package dsdgen

// Catalog sales/returns fan-out channel. One shared line generator drives both
// the catalog_sales (parent) and catalog_returns (child) fact tables; the engine
// (fact.go) selects which side each FactTable emits. Faithful port of
// CatalogSalesRowGenerator + CatalogReturnsRowGenerator and the matching Row
// classes' getValues() ordering.

// Catalog sales generator-column stream indices (iota order of
// CatalogSalesGeneratorColumn). These index the sales streamSet (sss).
const (
	csSoldDateSk = iota
	csSoldTimeSk
	csShipDateSk
	csBillCustomerSk
	csBillCdemoSk
	csBillHdemoSk
	csBillAddrSk
	csShipCustomerSk
	csShipCdemoSk
	csShipHdemoSk
	csShipAddrSk
	csCallCenterSk
	csCatalogPageSk
	csShipModeSk
	csWarehouseSk
	csSoldItemSk
	csPromoSk
	csOrderNumber
	csPricingQuantity
	csPricingWholesaleCost
	csPricingListPrice
	csPricingSalesPrice
	csPricingCouponAmt
	csPricingExtSalesPrice
	csPricingExtDiscountAmount
	csPricingExtWholesaleCost
	csPricingExtListPrice
	csPricingExtTax
	csPricingExtShipCost
	csPricingNetPaid
	csPricingNetPaidIncTax
	csPricingNetPaidIncShip
	csPricingNetPaidIncShipTax
	csPricingNetProfit
	csPricing
	csPermute
	csNulls
	crIsReturned
	csPermutation
)

// catalogSalesCols mirrors CatalogSalesGeneratorColumn (globalColumnNumber, seedsPerRow).
var catalogSalesCols = []GeneratorColumn{
	{75, 1},    // CS_SOLD_DATE_SK
	{76, 2},    // CS_SOLD_TIME_SK
	{77, 14},   // CS_SHIP_DATE_SK
	{78, 1},    // CS_BILL_CUSTOMER_SK
	{79, 1},    // CS_BILL_CDEMO_SK
	{80, 1},    // CS_BILL_HDEMO_SK
	{81, 1},    // CS_BILL_ADDR_SK
	{82, 2},    // CS_SHIP_CUSTOMER_SK
	{83, 1},    // CS_SHIP_CDEMO_SK
	{84, 1},    // CS_SHIP_HDEMO_SK
	{85, 1},    // CS_SHIP_ADDR_SK
	{86, 1},    // CS_CALL_CENTER_SK
	{87, 42},   // CS_CATALOG_PAGE_SK
	{88, 14},   // CS_SHIP_MODE_SK
	{89, 14},   // CS_WAREHOUSE_SK
	{90, 1},    // CS_SOLD_ITEM_SK
	{91, 14},   // CS_PROMO_SK
	{92, 1},    // CS_ORDER_NUMBER
	{93, 0},    // CS_PRICING_QUANTITY
	{94, 0},    // CS_PRICING_WHOLESALE_COST
	{95, 0},    // CS_PRICING_LIST_PRICE
	{96, 0},    // CS_PRICING_SALES_PRICE
	{97, 0},    // CS_PRICING_COUPON_AMT
	{98, 0},    // CS_PRICING_EXT_SALES_PRICE
	{99, 0},    // CS_PRICING_EXT_DISCOUNT_AMOUNT
	{100, 0},   // CS_PRICING_EXT_WHOLESALE_COST
	{101, 0},   // CS_PRICING_EXT_LIST_PRICE
	{102, 0},   // CS_PRICING_EXT_TAX
	{103, 0},   // CS_PRICING_EXT_SHIP_COST
	{104, 0},   // CS_PRICING_NET_PAID
	{105, 0},   // CS_PRICING_NET_PAID_INC_TAX
	{106, 0},   // CS_PRICING_NET_PAID_INC_SHIP
	{107, 0},   // CS_PRICING_NET_PAID_INC_SHIP_TAX
	{108, 0},   // CS_PRICING_NET_PROFIT
	{109, 112}, // CS_PRICING
	{110, 0},   // CS_PERMUTE
	{111, 28},  // CS_NULLS
	{112, 14},  // CR_IS_RETURNED
	{113, 0},   // CS_PERMUTATION
}

// Catalog returns generator-column stream indices (iota order of
// CatalogReturnsGeneratorColumn). These index the returns streamSet (srs).
const (
	crReturnedDateSk = iota
	crReturnedTimeSk
	crItemSk
	crRefundedCustomerSk
	crRefundedCdemoSk
	crRefundedHdemoSk
	crRefundedAddrSk
	crReturningCustomerSk
	crReturningCdemoSk
	crReturningHdemoSk
	crReturningAddrSk
	crCallCenterSk
	crCatalogPageSk
	crShipModeSk
	crWarehouseSk
	crReasonSk
	crOrderNumber
	crPricingQuantity
	crPricingNetPaid
	crPricingExtTax
	crPricingNetPaidIncTax
	crPricingFee
	crPricingExtShipCost
	crPricingRefundedCash
	crPricingReversedCharge
	crPricingStoreCredit
	crPricingNetLoss
	crNulls
	crPricing
)

// catalogReturnsCols mirrors CatalogReturnsGeneratorColumn.
var catalogReturnsCols = []GeneratorColumn{
	{46, 28}, // CR_RETURNED_DATE_SK
	{47, 28}, // CR_RETURNED_TIME_SK
	{48, 14}, // CR_ITEM_SK
	{49, 14}, // CR_REFUNDED_CUSTOMER_SK
	{50, 14}, // CR_REFUNDED_CDEMO_SK
	{51, 14}, // CR_REFUNDED_HDEMO_SK
	{52, 14}, // CR_REFUNDED_ADDR_SK
	{53, 28}, // CR_RETURNING_CUSTOMER_SK
	{54, 14}, // CR_RETURNING_CDEMO_SK
	{55, 14}, // CR_RETURNING_HDEMO_SK
	{56, 14}, // CR_RETURNING_ADDR_SK
	{57, 0},  // CR_CALL_CENTER_SK
	{58, 14}, // CR_CATALOG_PAGE_SK
	{59, 14}, // CR_SHIP_MODE_SK
	{60, 14}, // CR_WAREHOUSE_SK
	{61, 14}, // CR_REASON_SK
	{62, 0},  // CR_ORDER_NUMBER
	{63, 0},  // CR_PRICING_QUANTITY
	{64, 0},  // CR_PRICING_NET_PAID
	{65, 0},  // CR_PRICING_EXT_TAX
	{66, 0},  // CR_PRICING_NET_PAID_INC_TAX
	{67, 0},  // CR_PRICING_FEE
	{68, 0},  // CR_PRICING_EXT_SHIP_COST
	{69, 0},  // CR_PRICING_REFUNDED_CASH
	{70, 0},  // CR_PRICING_REVERSED_CHARGE
	{71, 0},  // CR_PRICING_STORE_CREDIT
	{72, 0},  // CR_PRICING_NET_LOSS
	{73, 28}, // CR_NULLS
	{74, 70}, // CR_PRICING
}

// Null parameters from Table.java.
const (
	catalogSalesNullBasis      = 100
	catalogSalesNotNullBitMap  = 0x28000
	catalogSalesFirstColumn    = 75 // CS_SOLD_DATE_SK global column number
	catalogReturnsNullBasis    = 400
	catalogReturnsNotNullBitMp = 0x10007
	catalogReturnsFirstColumn  = 46 // CR_RETURNED_DATE_SK global column number

	csGiftPercentage = 10 // CatalogSalesRowGenerator.GIFT_PERCENTAGE
	crReturnPercent  = 10 // CatalogReturnsRowGenerator.RETURN_PERCENT
)

// catalogOrderInfo holds the order-level (line-invariant) join keys for one
// catalog order, set once per ticket. Mirrors CatalogSalesRowGenerator.OrderInfo.
type catalogOrderInfo struct {
	soldDateSk     int64
	soldTimeSk     int64
	callCenterSk   int64
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

// catKeyOrNull renders a join-key surrogate column: empty for null or -1.
func catKeyOrNull(value int64, globalCol int, nullBitMap int64, firstCol int) any {
	if catalogIsNull(globalCol, nullBitMap, firstCol) || value == -1 {
		return nil
	}

	return value
}

// catValOrNull renders a non-key column (kept regardless of -1): empty if nulled.
func catValOrNull(value any, globalCol int, nullBitMap int64, firstCol int) any {
	if catalogIsNull(globalCol, nullBitMap, firstCol) {
		return nil
	}

	return value
}

func catalogIsNull(globalCol int, nullBitMap int64, firstCol int) bool {
	mask := int64(1) << (globalCol - firstCol)

	return nullBitMap&mask != 0
}

// newCatalogLineGen builds the shared per-stream line generator for the catalog
// channel. It mirrors CatalogSalesRowGenerator.generateRowAndChildRows.
func newCatalogLineGen(sss, srs *streamSet, sc *Scaling) FactLineGen {
	itemCount := int(sc.IDCount(TItem))

	var (
		itemPermutation []int
		julianDate      int64
		nextDateIndex   int64
		remainingItems  int
		ticketItemBase  int
		order           catalogOrderInfo
	)

	return func(ticket int64) ([]any, []any, bool) {
		if itemPermutation == nil {
			itemPermutation = MakePermutation(itemCount, sss.at(csPermute))
			// skipDaysUntilFirstRowOfChunk for the full-table (chunk 1) start:
			// julianDate stays at the data start date and nextDateIndex is the
			// first day's row count plus one.
			julianDate = int64(JulianDataStartDate)
			nextDateIndex = sc.RowCountForDate(TCatalogSales, julianDate) + 1
		}

		if remainingItems == 0 {
			order = generateCatalogOrderInfo(ticket, sss, sc, &julianDate, &nextDateIndex, order)
			ticketItemBase = GenerateUniformRandomInt(1, itemCount, sss.at(csSoldItemSk))
			remainingItems = GenerateUniformRandomInt(4, 14, sss.at(csOrderNumber))
		}

		nullBitMap := CreateNullBitMap(catalogSalesNullBasis, catalogSalesNotNullBitMap, sss.at(csNulls))

		// orders are shipped some number of days after they are ordered
		shippingLag := GenerateUniformRandomInt(csMinShipDelay, csMaxShipDelay, sss.at(csShipDateSk))
		shipDateSk := int64(-1)
		if order.soldDateSk != -1 {
			shipDateSk = order.soldDateSk + int64(shippingLag)
		}

		// items unique within an order: walk the permutation (1-based)
		ticketItemBase++
		if ticketItemBase > itemCount {
			ticketItemBase = 1
		}
		item := int64(PermutationEntry(itemPermutation, ticketItemBase))
		soldItemSk := MatchSurrogateKey(item, order.soldDateSk, TItem, sc)

		// catalog page must be from a catalog active at the time of the sale
		catalogPageSk := int64(-1)
		if order.soldDateSk != -1 {
			catalogPageSk = GenerateJoinKey(TCatalogSales, JCNone, sss.at(csCatalogPageSk), TCatalogPage, order.soldDateSk, sc)
		}

		shipModeSk := GenerateJoinKey(TCatalogSales, JCNone, sss.at(csShipModeSk), TShipMode, 1, sc)
		warehouseSk := GenerateJoinKey(TCatalogSales, JCNone, sss.at(csWarehouseSk), TWarehouse, 1, sc)
		promoSk := GenerateJoinKey(TCatalogSales, JCNone, sss.at(csPromoSk), TPromotion, 1, sc)
		pricing := GeneratePricingForSales(CatalogPricingLimits, sss.at(csPricing))

		nb := nullBitMap
		fc := catalogSalesFirstColumn
		sales := []any{
			catKeyOrNull(order.soldDateSk, 75, nb, fc),
			catKeyOrNull(order.soldTimeSk, 76, nb, fc),
			catKeyOrNull(shipDateSk, 77, nb, fc),
			catKeyOrNull(order.billCustomerSk, 78, nb, fc),
			catKeyOrNull(order.billCdemoSk, 79, nb, fc),
			catKeyOrNull(order.billHdemoSk, 80, nb, fc),
			catKeyOrNull(order.billAddrSk, 81, nb, fc),
			catKeyOrNull(order.shipCustomerSk, 82, nb, fc),
			catKeyOrNull(order.shipCdemoSk, 83, nb, fc),
			catKeyOrNull(order.shipHdemoSk, 84, nb, fc),
			catKeyOrNull(order.shipAddrSk, 85, nb, fc),
			catKeyOrNull(order.callCenterSk, 86, nb, fc),
			catKeyOrNull(catalogPageSk, 87, nb, fc),
			catKeyOrNull(shipModeSk, 88, nb, fc),
			catValOrNull(warehouseSk, 89, nb, fc),
			catKeyOrNull(soldItemSk, 90, nb, fc),
			catKeyOrNull(promoSk, 91, nb, fc),
			catValOrNull(order.orderNumber, 92, nb, fc),
			catValOrNull(pricing.Quantity, 93, nb, fc),
			catValOrNull(pricing.WholesaleCost, 94, nb, fc),
			catValOrNull(pricing.ListPrice, 95, nb, fc),
			catValOrNull(pricing.SalesPrice, 96, nb, fc),
			catValOrNull(pricing.ExtDiscountAmount, 99, nb, fc),
			catValOrNull(pricing.ExtSalesPrice, 98, nb, fc),
			catValOrNull(pricing.ExtWholesaleCost, 100, nb, fc),
			catValOrNull(pricing.ExtListPrice, 101, nb, fc),
			catValOrNull(pricing.ExtTax, 102, nb, fc),
			catValOrNull(pricing.CouponAmount, 97, nb, fc),
			catValOrNull(pricing.ExtShipCost, 103, nb, fc),
			catValOrNull(pricing.NetPaid, 104, nb, fc),
			catValOrNull(pricing.NetPaidIncludingTax, 105, nb, fc),
			catValOrNull(pricing.NetPaidIncludingShipping, 106, nb, fc),
			catValOrNull(pricing.NetPaidIncludingShippingAndTax, 107, nb, fc),
			catValOrNull(pricing.NetProfit, 108, nb, fc),
		}

		// Build the sales row record for the returns side.
		salesRow := catalogSalesRecord{
			soldItemSk:     soldItemSk,
			billCustomerSk: order.billCustomerSk,
			billCdemoSk:    order.billCdemoSk,
			billHdemoSk:    order.billHdemoSk,
			billAddrSk:     order.billAddrSk,
			shipCustomerSk: order.shipCustomerSk,
			shipCdemoSk:    order.shipCdemoSk,
			shipAddrSk:     order.shipAddrSk,
			callCenterSk:   order.callCenterSk,
			catalogPageSk:  catalogPageSk,
			shipDateSk:     shipDateSk,
			orderNumber:    order.orderNumber,
			pricing:        pricing,
		}

		var returns []any
		if GenerateUniformRandomInt(0, 99, sss.at(crIsReturned)) < crReturnPercent {
			returns = generateCatalogReturnsRow(srs, sc, salesRow)
		}

		remainingItems--

		return sales, returns, remainingItems == 0
	}
}

// catalogSalesRecord carries the fields of a generated catalog_sales row needed
// to build the paired catalog_returns row. Mirrors the subset of CatalogSalesRow
// read by CatalogReturnsRowGenerator.generateRow.
type catalogSalesRecord struct {
	soldItemSk     int64
	billCustomerSk int64
	billCdemoSk    int64
	billHdemoSk    int64
	billAddrSk     int64
	shipCustomerSk int64
	shipCdemoSk    int64
	shipAddrSk     int64
	callCenterSk   int64
	catalogPageSk  int64
	shipDateSk     int64
	orderNumber    int64
	pricing        Pricing
}

// generateCatalogOrderInfo ports CatalogSalesRowGenerator.generateOrderInfo. It
// advances julianDate/nextDateIndex for date-based row allocation, then draws
// the order-level join keys. prev is the previous order (its callCenterSk seeds
// the time-of-day join, exactly as the Java code reuses the prior orderInfo).
func generateCatalogOrderInfo(rowNumber int64, sss *streamSet, sc *Scaling, julianDate, nextDateIndex *int64, prev catalogOrderInfo) catalogOrderInfo {
	for rowNumber > *nextDateIndex {
		*julianDate++
		*nextDateIndex += sc.RowCountForDate(TCatalogSales, *julianDate)
	}

	soldDateSk := *julianDate
	soldTimeSk := GenerateJoinKey(TCatalogSales, JCNone, sss.at(csSoldTimeSk), TTimeDim, prev.callCenterSk, sc)
	callCenterSk := int64(-1)
	if soldDateSk != -1 {
		callCenterSk = GenerateJoinKey(TCatalogSales, JCNone, sss.at(csCallCenterSk), TCallCenter, soldDateSk, sc)
	}
	billCustomerSk := GenerateJoinKey(TCatalogSales, JCNone, sss.at(csBillCustomerSk), TCustomer, 1, sc)
	billCdemoSk := GenerateJoinKey(TCatalogSales, JCNone, sss.at(csBillCdemoSk), TCustomerDemographics, 1, sc)
	billHdemoSk := GenerateJoinKey(TCatalogSales, JCNone, sss.at(csBillHdemoSk), THouseholdDemographics, 1, sc)
	billAddrSk := GenerateJoinKey(TCatalogSales, JCNone, sss.at(csBillAddrSk), TCustomerAddress, 1, sc)

	giftPercentage := GenerateUniformRandomInt(0, 99, sss.at(csShipCustomerSk))
	shipCustomerSk := billCustomerSk
	shipCdemoSk := billCdemoSk
	shipHdemoSk := billHdemoSk
	shipAddrSk := billAddrSk
	if giftPercentage <= csGiftPercentage {
		shipCustomerSk = GenerateJoinKey(TCatalogSales, JCNone, sss.at(csShipCustomerSk), TCustomer, 2, sc)
		shipCdemoSk = GenerateJoinKey(TCatalogSales, JCNone, sss.at(csShipCdemoSk), TCustomerDemographics, 2, sc)
		shipHdemoSk = GenerateJoinKey(TCatalogSales, JCNone, sss.at(csShipHdemoSk), THouseholdDemographics, 2, sc)
		shipAddrSk = GenerateJoinKey(TCatalogSales, JCNone, sss.at(csShipAddrSk), TCustomerAddress, 2, sc)
	}

	return catalogOrderInfo{
		soldDateSk:     soldDateSk,
		soldTimeSk:     soldTimeSk,
		callCenterSk:   callCenterSk,
		billCustomerSk: billCustomerSk,
		billCdemoSk:    billCdemoSk,
		billHdemoSk:    billHdemoSk,
		billAddrSk:     billAddrSk,
		shipCustomerSk: shipCustomerSk,
		shipCdemoSk:    shipCdemoSk,
		shipHdemoSk:    shipHdemoSk,
		shipAddrSk:     shipAddrSk,
		orderNumber:    rowNumber,
	}
}

// generateCatalogReturnsRow ports CatalogReturnsRowGenerator.generateRow, drawing
// on the returns streamSet (srs) and reusing the sales row's keys/pricing.
func generateCatalogReturnsRow(srs *streamSet, sc *Scaling, sales catalogSalesRecord) []any {
	nullBitMap := CreateNullBitMap(catalogReturnsNullBasis, catalogReturnsNotNullBitMp, srs.at(crNulls))

	returningCustomerSk := GenerateJoinKey(TCatalogReturns, JCNone, srs.at(crReturningCustomerSk), TCustomer, 2, sc)
	returningCdemoSk := GenerateJoinKey(TCatalogReturns, JCNone, srs.at(crReturningCdemoSk), TCustomerDemographics, 2, sc)
	returningHdemoSk := GenerateJoinKey(TCatalogReturns, JCNone, srs.at(crReturningHdemoSk), THouseholdDemographics, 2, sc)
	returningAddrSk := GenerateJoinKey(TCatalogReturns, JCNone, srs.at(crReturningAddrSk), TCustomerAddress, 2, sc)
	if GenerateUniformRandomInt(0, 99, srs.at(crReturningCustomerSk)) < csGiftPercentage {
		returningCustomerSk = sales.shipCustomerSk
		returningCdemoSk = sales.shipCdemoSk
		// skip returningHdemoSk: not present on the sales record
		returningAddrSk = sales.shipAddrSk
	}

	quantity := sales.pricing.Quantity
	if sales.pricing.Quantity != -1 {
		quantity = GenerateUniformRandomInt(1, quantity, srs.at(crPricing))
	}
	pricing := GeneratePricingForReturns(srs.at(crPricing), quantity, sales.pricing)

	returnedDateSk := GenerateJoinKey(TCatalogReturns, JCNone, srs.at(crReturnedDateSk), TDateDim, sales.shipDateSk, sc)
	returnedTimeSk := GenerateJoinKey(TCatalogReturns, JCNone, srs.at(crReturnedTimeSk), TTimeDim, 1, sc)
	shipModeSk := GenerateJoinKey(TCatalogReturns, JCNone, srs.at(crShipModeSk), TShipMode, 1, sc)
	warehouseSk := GenerateJoinKey(TCatalogReturns, JCNone, srs.at(crWarehouseSk), TWarehouse, 1, sc)
	reasonSk := GenerateJoinKey(TCatalogReturns, JCNone, srs.at(crReasonSk), TReason, 1, sc)

	nb := nullBitMap
	fc := catalogReturnsFirstColumn

	return []any{
		catKeyOrNull(returnedDateSk, 46, nb, fc),
		catKeyOrNull(returnedTimeSk, 47, nb, fc),
		catKeyOrNull(sales.soldItemSk, 48, nb, fc),
		catKeyOrNull(sales.billCustomerSk, 49, nb, fc),
		catKeyOrNull(sales.billCdemoSk, 50, nb, fc),
		catKeyOrNull(sales.billHdemoSk, 51, nb, fc),
		catKeyOrNull(sales.billAddrSk, 52, nb, fc),
		catKeyOrNull(returningCustomerSk, 53, nb, fc),
		catKeyOrNull(returningCdemoSk, 54, nb, fc),
		catKeyOrNull(returningHdemoSk, 55, nb, fc),
		catKeyOrNull(returningAddrSk, 56, nb, fc),
		catKeyOrNull(sales.callCenterSk, 57, nb, fc),
		catKeyOrNull(sales.catalogPageSk, 58, nb, fc),
		catKeyOrNull(shipModeSk, 59, nb, fc),
		catKeyOrNull(warehouseSk, 60, nb, fc),
		catKeyOrNull(reasonSk, 61, nb, fc),
		catValOrNull(sales.orderNumber, 62, nb, fc),
		catValOrNull(pricing.Quantity, 63, nb, fc),
		catValOrNull(pricing.NetPaid, 64, nb, fc),
		catValOrNull(pricing.ExtTax, 65, nb, fc),
		catValOrNull(pricing.NetPaidIncludingTax, 66, nb, fc),
		catValOrNull(pricing.Fee, 67, nb, fc),
		catValOrNull(pricing.ExtShipCost, 68, nb, fc),
		catValOrNull(pricing.RefundedCash, 69, nb, fc),
		catValOrNull(pricing.ReversedCharge, 70, nb, fc),
		catValOrNull(pricing.StoreCredit, 71, nb, fc),
		catValOrNull(pricing.NetLoss, 72, nb, fc),
	}
}

var catalogTicketCount = func(sf float64) int64 { return NewScaling(sf).RowCount(TCatalogSales) }

// CatalogSales is the catalog_sales (parent/sales) fact table.
var CatalogSales = &FactTable{
	Name: "catalog_sales",
	ID:   TCatalogSales,
	Columns: []string{
		"cs_sold_date_sk", "cs_sold_time_sk", "cs_ship_date_sk", "cs_bill_customer_sk",
		"cs_bill_cdemo_sk", "cs_bill_hdemo_sk", "cs_bill_addr_sk", "cs_ship_customer_sk",
		"cs_ship_cdemo_sk", "cs_ship_hdemo_sk", "cs_ship_addr_sk", "cs_call_center_sk",
		"cs_catalog_page_sk", "cs_ship_mode_sk", "cs_warehouse_sk", "cs_item_sk",
		"cs_promo_sk", "cs_order_number", "cs_quantity", "cs_wholesale_cost",
		"cs_list_price", "cs_sales_price", "cs_ext_discount_amt", "cs_ext_sales_price",
		"cs_ext_wholesale_cost", "cs_ext_list_price", "cs_ext_tax", "cs_coupon_amt",
		"cs_ext_ship_cost", "cs_net_paid", "cs_net_paid_inc_tax", "cs_net_paid_inc_ship",
		"cs_net_paid_inc_ship_tax", "cs_net_profit",
	},
	emitReturns: false,
	salesCols:   catalogSalesCols,
	returnsCols: catalogReturnsCols,
	TicketCount: catalogTicketCount,
	newLineGen:  newCatalogLineGen,
}

// CatalogReturns is the catalog_returns (child/returns) fact table.
var CatalogReturns = &FactTable{
	Name: "catalog_returns",
	ID:   TCatalogReturns,
	Columns: []string{
		"cr_returned_date_sk", "cr_returned_time_sk", "cr_item_sk", "cr_refunded_customer_sk",
		"cr_refunded_cdemo_sk", "cr_refunded_hdemo_sk", "cr_refunded_addr_sk", "cr_returning_customer_sk",
		"cr_returning_cdemo_sk", "cr_returning_hdemo_sk", "cr_returning_addr_sk", "cr_call_center_sk",
		"cr_catalog_page_sk", "cr_ship_mode_sk", "cr_warehouse_sk", "cr_reason_sk",
		"cr_order_number", "cr_return_quantity", "cr_return_amount", "cr_return_tax",
		"cr_return_amt_inc_tax", "cr_fee", "cr_return_ship_cost", "cr_refunded_cash",
		"cr_reversed_charge", "cr_store_credit", "cr_net_loss",
	},
	emitReturns: true,
	salesCols:   catalogSalesCols,
	returnsCols: catalogReturnsCols,
	TicketCount: catalogTicketCount,
	newLineGen:  newCatalogLineGen,
}
