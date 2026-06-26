package dsdgen

// Pricing holds the money columns shared by the sales and returns fact tables.
// Field math and RNG draw order mirror type/Pricing.java exactly (which copies
// pricing.c), so the consumed seed sequence is byte-identical to dsdgen.
type Pricing struct {
	WholesaleCost                  Decimal
	ListPrice                      Decimal
	SalesPrice                     Decimal
	Quantity                       int
	ExtDiscountAmount              Decimal
	ExtSalesPrice                  Decimal
	ExtWholesaleCost               Decimal
	ExtListPrice                   Decimal
	TaxPercent                     Decimal
	ExtTax                         Decimal
	CouponAmount                   Decimal
	ShipCost                       Decimal
	ExtShipCost                    Decimal
	NetPaid                        Decimal
	NetPaidIncludingTax            Decimal
	NetPaidIncludingShipping       Decimal
	NetPaidIncludingShippingAndTax Decimal
	NetProfit                      Decimal
	RefundedCash                   Decimal
	ReversedCharge                 Decimal
	StoreCredit                    Decimal
	Fee                            Decimal
	NetLoss                        Decimal
}

// PricingLimits bound the random draws for one sales channel. Values from
// Pricing.LIMITS_PER_COLUMN (catalog from CatalogSalesRowGenerator constants).
type PricingLimits struct {
	MaxQuantitySold  int
	MaxMarkup        Decimal
	MaxDiscount      Decimal
	MaxWholesaleCost Decimal
}

// Per-channel pricing limits (store / catalog / web).
var (
	StorePricingLimits   = PricingLimits{100, DecimalOne, DecimalOne, DecimalOneHundred}
	CatalogPricingLimits = PricingLimits{100, Decimal{2, 200}, Decimal{2, 100}, Decimal{2, 10000}}
	WebPricingLimits     = PricingLimits{100, Decimal{2, 200}, DecimalOne, DecimalOneHundred}
)

const pricingQuantityMin = 1

var (
	pricingMarkupMin   = Decimal{Precision: 2, Number: 0}
	pricingDiscountMin = Decimal{Precision: 2, Number: 0}
)

// GeneratePricingForSales computes the pricing for a sales-fact line, drawing on
// s in the exact order of generatePricingForSalesTable.
func GeneratePricingForSales(limits PricingLimits, s *RNStream) Pricing {
	quantity := GenerateUniformRandomInt(pricingQuantityMin, limits.MaxQuantitySold, s)
	dq := DecimalFromInteger(quantity)
	wholesaleCost := GenerateUniformRandomDecimal(Decimal{2, 100}, limits.MaxWholesaleCost, s)
	extWholesaleCost := MulDecimal(dq, wholesaleCost)

	markup := AddDecimal(GenerateUniformRandomDecimal(pricingMarkupMin, limits.MaxMarkup, s), DecimalOne)
	listPrice := MulDecimal(wholesaleCost, markup)

	discount := AddDecimal(NegateDecimal(GenerateUniformRandomDecimal(pricingDiscountMin, limits.MaxDiscount, s)), DecimalOne)
	salesPrice := MulDecimal(listPrice, discount)
	extListPrice := MulDecimal(listPrice, dq)
	extSalesPrice := MulDecimal(salesPrice, dq)
	extDiscountAmount := SubDecimal(extListPrice, extSalesPrice)

	coupon := GenerateUniformRandomDecimal(DecimalZero, DecimalOne, s)
	couponUsage := GenerateUniformRandomInt(1, 100, s)
	couponAmount := DecimalZero
	if couponUsage <= 20 { // 20% of sales use a coupon
		couponAmount = MulDecimal(extSalesPrice, coupon)
	}
	netPaid := SubDecimal(extSalesPrice, couponAmount)

	shipping := GenerateUniformRandomDecimal(DecimalZero, DecimalOneHalf, s)
	shipCost := MulDecimal(listPrice, shipping)
	extShipCost := MulDecimal(shipCost, dq)
	netPaidIncludingShipping := AddDecimal(netPaid, extShipCost)
	taxPercent := GenerateUniformRandomDecimal(DecimalZero, DecimalNinePct, s)
	extTax := MulDecimal(netPaid, taxPercent)
	netPaidIncludingTax := AddDecimal(netPaid, extTax)
	netPaidIncludingShippingAndTax := AddDecimal(netPaidIncludingShipping, extTax)
	netProfit := SubDecimal(netPaid, extWholesaleCost)

	return Pricing{
		WholesaleCost: wholesaleCost, ListPrice: listPrice, SalesPrice: salesPrice, Quantity: quantity,
		ExtDiscountAmount: extDiscountAmount, ExtSalesPrice: extSalesPrice, ExtWholesaleCost: extWholesaleCost,
		ExtListPrice: extListPrice, TaxPercent: taxPercent, ExtTax: extTax, CouponAmount: couponAmount,
		ShipCost: shipCost, ExtShipCost: extShipCost, NetPaid: netPaid, NetPaidIncludingTax: netPaidIncludingTax,
		NetPaidIncludingShipping: netPaidIncludingShipping, NetPaidIncludingShippingAndTax: netPaidIncludingShippingAndTax,
		NetProfit: netProfit,
		// refunded/reversed/credit/fee/loss are zero for sales.
	}
}

// GeneratePricingForReturns computes the pricing for a returns-fact line given
// the returned quantity and the original sale's pricing. Mirrors
// generatePricingForReturnsTable.
func GeneratePricingForReturns(s *RNStream, quantity int, base Pricing) Pricing {
	dq := DecimalFromInteger(quantity)
	extWholesaleCost := MulDecimal(dq, base.WholesaleCost)
	extListPrice := MulDecimal(base.ListPrice, dq)
	extSalesPrice := MulDecimal(base.SalesPrice, dq)
	netPaid := extSalesPrice

	shipping := GenerateUniformRandomDecimal(DecimalZero, DecimalOneHalf, s)
	shipCost := MulDecimal(base.ListPrice, shipping)
	extShipCost := MulDecimal(shipCost, dq)
	netPaidIncludingShipping := AddDecimal(netPaid, extShipCost)
	extTax := MulDecimal(netPaid, base.TaxPercent)
	netPaidIncludingTax := AddDecimal(netPaid, extTax)
	netPaidIncludingShippingAndTax := AddDecimal(netPaidIncludingShipping, extTax)
	netProfit := SubDecimal(netPaid, extWholesaleCost)

	// Split the returned amount across cash, reversed charge and store credit.
	cashPct := DecimalFromInteger(GenerateUniformRandomInt(0, 100, s))
	refundedCash := MulDecimal(DivDecimal(cashPct, DecimalOneHundred), netPaid)

	creditPct := DivDecimal(DecimalFromInteger(GenerateUniformRandomInt(1, 100, s)), DecimalOneHundred)
	reversedCharge := MulDecimal(creditPct, SubDecimal(netPaid, refundedCash))

	storeCredit := SubDecimal(SubDecimal(netPaid, reversedCharge), refundedCash)

	fee := GenerateUniformRandomDecimal(DecimalOneHalf, DecimalOneHundred, s)

	netLoss := SubDecimal(netPaidIncludingShippingAndTax, storeCredit)
	netLoss = SubDecimal(netLoss, refundedCash)
	netLoss = SubDecimal(netLoss, reversedCharge)
	netLoss = AddDecimal(netLoss, fee)

	return Pricing{
		WholesaleCost: base.WholesaleCost, ListPrice: base.ListPrice, SalesPrice: base.SalesPrice, Quantity: quantity,
		ExtDiscountAmount: base.ExtDiscountAmount, ExtSalesPrice: extSalesPrice, ExtWholesaleCost: extWholesaleCost,
		ExtListPrice: extListPrice, TaxPercent: base.TaxPercent, ExtTax: extTax, CouponAmount: base.CouponAmount,
		ShipCost: shipCost, ExtShipCost: extShipCost, NetPaid: netPaid, NetPaidIncludingTax: netPaidIncludingTax,
		NetPaidIncludingShipping: netPaidIncludingShipping, NetPaidIncludingShippingAndTax: netPaidIncludingShippingAndTax,
		NetProfit: netProfit, RefundedCash: refundedCash, ReversedCharge: reversedCharge, StoreCredit: storeCredit,
		Fee: fee, NetLoss: netLoss,
	}
}
