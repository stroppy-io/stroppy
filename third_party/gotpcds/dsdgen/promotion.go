package dsdgen

// Promotion column stream layout (table-local indices into the streamSet).
// Global column numbers and per-row seed counts come from
// PromotionGeneratorColumn.java.
const (
	pPromoSk = iota
	pPromoID
	pStartDateID
	pEndDateID
	pItemSk
	pCost
	pResponseTarget
	pPromoName
	pChannelDmail
	pChannelEmail
	pChannelCatalog
	pChannelTv
	pChannelRadio
	pChannelPress
	pChannelEvent
	pChannelDemo
	pChannelDetails
	pPurpose
	pDiscountActive
	pNulls
)

var promotionCols = []GeneratorColumn{
	pPromoSk:        {GlobalColumnNumber: 228, SeedsPerRow: 1},
	pPromoID:        {GlobalColumnNumber: 229, SeedsPerRow: 1},
	pStartDateID:    {GlobalColumnNumber: 230, SeedsPerRow: 1},
	pEndDateID:      {GlobalColumnNumber: 231, SeedsPerRow: 1},
	pItemSk:         {GlobalColumnNumber: 232, SeedsPerRow: 1},
	pCost:           {GlobalColumnNumber: 233, SeedsPerRow: 1},
	pResponseTarget: {GlobalColumnNumber: 234, SeedsPerRow: 1},
	pPromoName:      {GlobalColumnNumber: 235, SeedsPerRow: 1},
	pChannelDmail:   {GlobalColumnNumber: 236, SeedsPerRow: 1},
	pChannelEmail:   {GlobalColumnNumber: 237, SeedsPerRow: 1},
	pChannelCatalog: {GlobalColumnNumber: 238, SeedsPerRow: 1},
	pChannelTv:      {GlobalColumnNumber: 239, SeedsPerRow: 1},
	pChannelRadio:   {GlobalColumnNumber: 240, SeedsPerRow: 1},
	pChannelPress:   {GlobalColumnNumber: 241, SeedsPerRow: 1},
	pChannelEvent:   {GlobalColumnNumber: 242, SeedsPerRow: 1},
	pChannelDemo:    {GlobalColumnNumber: 243, SeedsPerRow: 1},
	pChannelDetails: {GlobalColumnNumber: 244, SeedsPerRow: 100},
	pPurpose:        {GlobalColumnNumber: 245, SeedsPerRow: 1},
	pDiscountActive: {GlobalColumnNumber: 246, SeedsPerRow: 1},
	pNulls:          {GlobalColumnNumber: 247, SeedsPerRow: 2},
}

// Promotion null parameters (Table.PROMOTION): nullBasisPoints 200, notNullBitMap
// 0x3 (P_PROMO_SK and P_PROMO_ID are never nulled).
const (
	promotionNullBasis     = 200
	promotionNotNullBitMap = 0x3
	pFirstColumnGlobalNum  = 228 // P_PROMO_SK

	promoStartMin        = -720
	promoStartMax        = 100
	promoLengthMin       = 1
	promoLengthMax       = 60
	promoNameLength      = 5
	promoDetailLengthMin = 20
	promoDetailLengthMax = 60
)

// pIsNull reports whether the output column at table-local generator index
// localIdx is nulled by the row's bitmap (same bit offset as
// TableRowWithNulls.isNull).
func pIsNull(nullBitMap int64, localIdx int) bool {
	off := promotionCols[localIdx].GlobalColumnNumber - pFirstColumnGlobalNum

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

func boolField(b bool) string {
	if b {
		return "Y"
	}

	return "N"
}

// promotionRowCount mirrors Table.PROMOTION ScalingInfo (LOGARITHMIC, multiplier
// 0, no keepsHistory): the row count is read verbatim from the per-scale table.
func promotionRowCount(sf float64) int64 { return NewScaling(sf).RowCount(TPromotion) }

// Promotion is the TPC-DS promotion table: a flat dimension. Mirrors
// PromotionRowGenerator: draws on P_NULLS, P_START_DATE_ID, P_END_DATE_ID,
// P_ITEM_SK (join to ITEM), P_PROMO_NAME (no draw), P_CHANNEL_DMAIL (flags) and
// P_CHANNEL_DETAILS in that order, producing the 19 output columns of
// PromotionRow.getValues.
var Promotion = &Table{
	Name: "promotion",
	ID:   TPromotion,
	Columns: []string{
		"p_promo_sk", "p_promo_id", "p_start_date_sk", "p_end_date_sk", "p_item_sk",
		"p_cost", "p_response_target", "p_promo_name",
		"p_channel_dmail", "p_channel_email", "p_channel_catalog", "p_channel_tv",
		"p_channel_radio", "p_channel_press", "p_channel_event", "p_channel_demo",
		"p_channel_details", "p_purpose", "p_discount_active",
	},
	Cols:     promotionCols,
	RowCount: promotionRowCount,
	Row: func(rowNumber int64, ss *streamSet, sc *Scaling) []any {
		nullBitMap := CreateNullBitMap(promotionNullBasis, promotionNotNullBitMap, ss.at(pNulls))

		promoSk := rowNumber
		promoID := MakeBusinessKey(rowNumber)
		startDateID := int64(JulianDateMinimum) + int64(GenerateUniformRandomInt(promoStartMin, promoStartMax, ss.at(pStartDateID)))
		endDateID := startDateID + int64(GenerateUniformRandomInt(promoLengthMin, promoLengthMax, ss.at(pEndDateID)))

		itemSk := GenerateJoinKey(TPromotion, JCNone, ss.at(pItemSk), TItem, 1, sc)

		cost := Decimal{Precision: 2, Number: 100000}
		responseTarget := int64(1)
		promoName := generateWord(rowNumber, promoNameLength, syllablesDist)

		flags := GenerateUniformRandomInt(0, 511, ss.at(pChannelDmail))
		dmail := flags&0x01 != 0
		flags <<= 1
		email := flags&0x01 != 0
		flags <<= 1
		catalog := flags&0x01 != 0
		flags <<= 1
		tv := flags&0x01 != 0
		flags <<= 1
		radio := flags&0x01 != 0
		flags <<= 1
		press := flags&0x01 != 0
		flags <<= 1
		event := flags&0x01 != 0
		flags <<= 1
		demo := flags&0x01 != 0
		flags <<= 1
		discountActive := flags&0x01 != 0

		channelDetails := GenerateRandomText(promoDetailLengthMin, promoDetailLengthMax, ss.at(pChannelDetails))
		purpose := "Unknown"

		// Output values in PromotionRow.getValues order. A nulled column becomes
		// nil; key columns also become nil when the surrogate key is -1.
		vals := []any{
			promoSk,                   // p_promo_sk (key)
			promoID,                   // p_promo_id
			startDateID,               // p_start_date_sk (key-null rule)
			endDateID,                 // p_end_date_sk (key-null rule)
			itemSk,                    // p_item_sk (key-null rule)
			cost,                      // p_cost
			responseTarget,            // p_response_target
			promoName,                 // p_promo_name
			boolField(dmail),          // p_channel_dmail
			boolField(email),          // p_channel_email
			boolField(catalog),        // p_channel_catalog
			boolField(tv),             // p_channel_tv
			boolField(radio),          // p_channel_radio
			boolField(press),          // p_channel_press
			boolField(event),          // p_channel_event
			boolField(demo),           // p_channel_demo
			channelDetails,            // p_channel_details
			purpose,                   // p_purpose
			boolField(discountActive), // p_discount_active
		}

		// Map output column index -> table-local generator column index for the
		// null check, and flag which outputs use the key-null (value == -1) rule.
		nullCol := []int{
			pPromoSk, pPromoID, pStartDateID, pEndDateID, pItemSk,
			pCost, pResponseTarget, pPromoName,
			pChannelDmail, pChannelEmail, pChannelCatalog, pChannelTv,
			pChannelRadio, pChannelPress, pChannelEvent, pChannelDemo,
			pChannelDetails, pPurpose, pDiscountActive,
		}
		keyCols := map[int]int64{0: promoSk, 2: startDateID, 3: endDateID, 4: itemSk}
		for i := range vals {
			if v, isKey := keyCols[i]; isKey && v == -1 {
				vals[i] = nil

				continue
			}
			if pIsNull(nullBitMap, nullCol[i]) {
				vals[i] = nil
			}
		}

		return vals
	},
}
