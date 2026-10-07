package dsdgen

import (
	"fmt"
	"strconv"
)

// Item column stream layout (table-local indices into the streamSet). Global
// column numbers and per-row seed counts come from ItemGeneratorColumn.java, in
// enum order so consumeRemaining keeps every per-row seed budget aligned with
// dsdgen even for columns that are never drawn from directly.
const (
	iItemSk = iota
	iItemID
	iRecStartDateID
	iRecEndDateID
	iItemDesc
	iCurrentPrice
	iWholesaleCost
	iBrandID
	iBrand
	iClassID
	iClass
	iCategoryID
	iCategory
	iManufactID
	iManufact
	iSize
	iFormulation
	iColor
	iUnits
	iContainer
	iManagerID
	iProductName
	iNulls
	iScd
	iPromoSk
)

var itemCols = []GeneratorColumn{
	iItemSk:         {GlobalColumnNumber: 203, SeedsPerRow: 1},
	iItemID:         {GlobalColumnNumber: 204, SeedsPerRow: 1},
	iRecStartDateID: {GlobalColumnNumber: 205, SeedsPerRow: 1},
	iRecEndDateID:   {GlobalColumnNumber: 206, SeedsPerRow: 2},
	iItemDesc:       {GlobalColumnNumber: 207, SeedsPerRow: 200},
	iCurrentPrice:   {GlobalColumnNumber: 208, SeedsPerRow: 2},
	iWholesaleCost:  {GlobalColumnNumber: 209, SeedsPerRow: 1},
	iBrandID:        {GlobalColumnNumber: 210, SeedsPerRow: 1},
	iBrand:          {GlobalColumnNumber: 211, SeedsPerRow: 1},
	iClassID:        {GlobalColumnNumber: 212, SeedsPerRow: 1},
	iClass:          {GlobalColumnNumber: 213, SeedsPerRow: 1},
	iCategoryID:     {GlobalColumnNumber: 214, SeedsPerRow: 1},
	iCategory:       {GlobalColumnNumber: 215, SeedsPerRow: 1},
	iManufactID:     {GlobalColumnNumber: 216, SeedsPerRow: 2},
	iManufact:       {GlobalColumnNumber: 217, SeedsPerRow: 1},
	iSize:           {GlobalColumnNumber: 218, SeedsPerRow: 1},
	iFormulation:    {GlobalColumnNumber: 219, SeedsPerRow: 50},
	iColor:          {GlobalColumnNumber: 220, SeedsPerRow: 1},
	iUnits:          {GlobalColumnNumber: 221, SeedsPerRow: 1},
	iContainer:      {GlobalColumnNumber: 222, SeedsPerRow: 1},
	iManagerID:      {GlobalColumnNumber: 223, SeedsPerRow: 2},
	iProductName:    {GlobalColumnNumber: 224, SeedsPerRow: 1},
	iNulls:          {GlobalColumnNumber: 225, SeedsPerRow: 2},
	iScd:            {GlobalColumnNumber: 226, SeedsPerRow: 1},
	iPromoSk:        {GlobalColumnNumber: 227, SeedsPerRow: 2},
}

// item null parameters (Table.ITEM): nullBasisPoints 50, notNullBitMap 0x0B
// (I_ITEM_SK, I_ITEM_ID and I_REC_END_DATE are never nulled).
const (
	itemNullBasis      = 50
	itemNotNullBitMap  = 0x0B
	iFirstColumnGlobal = 203 // I_ITEM_SK
)

// item text-generation bounds and constants, transcribed from ItemRowGenerator.java.
const (
	itemRowSizeProductName = 50
	itemRowSizeItemDesc    = 200
	itemRowSizeManufact    = 50
	itemRowSizeFormulation = 20
	itemPromoPercentage    = 20
)

var (
	itemMinMarkdownPct = Decimal{Precision: 2, Number: 30}
	itemMaxMarkdownPct = Decimal{Precision: 2, Number: 90}
)

// Item distributions. Manager/manufact id ranges carry three int value columns
// (index, min, max) and four weight columns; only the UNIFIED weight (ordinal 0)
// and the min/max value columns (1 and 2) are used. Category-class brand counts
// come from one of ten per-category .dst files.
var (
	itemManagerIDDist   = mustLoadStringValues("item_manager_id.dst", 3, 4)
	itemManufactIDDist  = mustLoadStringValues("item_manufact_id.dst", 3, 4)
	itemSizesDist       = mustLoadStringValues("sizes.dst", 1, 3)
	itemColorsDist      = mustLoadStringValues("colors.dst", 1, 5)
	itemUnitsDist       = mustLoadStringValues("units.dst", 1, 1)
	itemBrandSyllables  = mustLoadStringValues("brand_syllables.dst", 1, 1)
	itemCategoriesDist  = mustLoadStringValues("categories.dst", 3, 1)
	itemCategoryClasses = []*StringValuesDistribution{
		mustLoadStringValues("women_class.dst", 2, 1),
		mustLoadStringValues("men_class.dst", 2, 1),
		mustLoadStringValues("children_class.dst", 2, 1),
		mustLoadStringValues("shoe_class.dst", 2, 1),
		mustLoadStringValues("music_class.dst", 2, 1),
		mustLoadStringValues("jewelry_class.dst", 2, 1),
		mustLoadStringValues("home_class.dst", 2, 1),
		mustLoadStringValues("sport_class.dst", 2, 1),
		mustLoadStringValues("book_class.dst", 2, 1),
		mustLoadStringValues("electronic_class.dst", 2, 1),
	}
	itemCurrentPriceDist = mustLoadStringValues("item_current_price.dst", 3, 4)
)

// idWeights ordinals (ItemsDistributions.IdWeights). dsdgen always uses UNIFIED.
const idWeightsUnified = 0

// sizeWeights ordinals (ItemsDistributions.SizeWeights).
const (
	sizeWeightsNoSize = 1
	sizeWeightsSized  = 2
)

// colorsWeights ordinal SKEWED (ItemsDistributions.ColorsWeights).
const colorsWeightsSkewed = 1

func atoiOrPanic(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic("dsdgen: item: bad integer " + s)
	}

	return n
}

// pickItemIDRange draws a [min,max] integer range from a 3-value/4-weight id
// distribution using the UNIFIED weight column. Mirrors pickRandomManagerIdRange
// / pickRandomManufactIdRange.
func pickItemIDRange(d *StringValuesDistribution, s *RNStream) (int, int) {
	idx := d.PickRandomIndex(idWeightsUnified, s)

	return atoiOrPanic(d.ValueAtIndex(1, idx)), atoiOrPanic(d.ValueAtIndex(2, idx))
}

// iIsNull reports whether the output column at table-local index localIdx is
// nulled by the row's bitmap, using the bit offset (globalColumnNumber - first).
func iIsNull(nullBitMap int64, localIdx int) bool {
	off := itemCols[localIdx].GlobalColumnNumber - iFirstColumnGlobal

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// itemRow holds one item row's fully resolved fields (after SCD inheritance).
type itemRow struct {
	nullBitMap                  int64
	itemID                      string
	recStart, recEnd            int64
	desc                        string
	currentPrice, wholesale     Decimal
	brandID                     int64
	brand                       string
	classID                     int64
	class                       string
	categoryID                  int64
	category                    string
	manufactID                  int64
	manufact, size, formulation string
	color, units, container     string
	managerID                   int64
	productName                 string
	promoSk                     int64
}

// computeItem generates one item row by drawing on ss in dsdgen's exact order.
// For a revision row it reconstructs the immediately preceding row's values on an
// independent streamSet and inherits the unchanged fields, so the result depends
// only on rowNumber (partition-safe, no shared state). Recursion depth is bounded
// by 2 since the new-key row is at most two rows back.
func computeItem(rowNumber int64, ss *streamSet, sc *Scaling) itemRow {
	var r itemRow
	r.nullBitMap = CreateNullBitMap(itemNullBasis, itemNotNullBitMap, ss.at(iNulls))

	managerMin, managerMax := pickItemIDRange(itemManagerIDDist, ss.at(iManagerID))
	r.managerID = GenerateUniformRandomKey(int64(managerMin), int64(managerMax), ss.at(iManagerID))

	scd := ComputeScdKey(TItem, rowNumber)
	r.itemID = scd.BusinessKey
	r.recStart = scd.StartDate
	r.recEnd = scd.EndDate
	isNewKey := scd.IsNewKey

	// A revision inherits unchanged fields from the immediately preceding row
	// (the previous revision of the same key), reconstructed independently so the
	// result depends only on rowNumber. New-key rows always take the drawn value
	// (getValueForSlowlyChangingDimension returns the new value when isNewKey), so
	// they need no predecessor: recursion only happens for revision rows and the
	// chain bottoms out at the new-key row at most two rows back (depth <= 2).
	var prev itemRow
	if !isNewKey {
		base := rowNumber - 1
		bss := newStreamSet(itemCols)
		bss.skipRows(base - 1)
		prev = computeItem(base, bss, sc)
	}

	fieldChangeFlags := ss.at(iScd).NextRandom()
	scdField := func(old, drawn any) any {
		if isNewKey {
			return drawn
		}

		return SCDValue(int(fieldChangeFlags), false, old, drawn)
	}

	desc := GenerateRandomText(1, itemRowSizeItemDesc, ss.at(iItemDesc))
	r.desc = scdField(prev.desc, desc).(string)
	fieldChangeFlags >>= 1

	// There is a bug in the C code that always chooses the new record for current price.
	cpMin, cpMax := pickCurrentPriceRange(ss.at(iCurrentPrice))
	r.currentPrice = GenerateUniformRandomDecimal(cpMin, cpMax, ss.at(iCurrentPrice))
	fieldChangeFlags >>= 1

	markdown := GenerateUniformRandomDecimal(itemMinMarkdownPct, itemMaxMarkdownPct, ss.at(iWholesaleCost))
	wholesale := MulDecimal(r.currentPrice, markdown)
	r.wholesale = scdField(prev.wholesale, wholesale).(Decimal)
	fieldChangeFlags >>= 1

	categoryIndex := itemCategoriesDist.PickRandomIndex(0, ss.at(iCategory))
	r.categoryID = int64(categoryIndex) + 1
	r.category = itemCategoriesDist.ValueAtIndex(0, categoryIndex)

	classDist := itemCategoryClasses[categoryIndex]
	classIndex := classDist.PickRandomIndex(0, ss.at(iClass))
	r.class = classDist.ValueAtIndex(0, classIndex)
	newClassID := int64(classIndex) + 1
	r.classID = scdField(prev.classID, newClassID).(int64)
	fieldChangeFlags >>= 1

	brandCount := int64(atoiOrPanic(classDist.ValueAtIndex(1, classIndex)))
	brandID := rowNumber%brandCount + 1
	r.brand = generateWord(r.categoryID*10+newClassID, 45, itemBrandSyllables)
	r.brand += fmt.Sprintf(" #%d", brandID)
	brandID += (r.categoryID*1000 + newClassID) * 1000
	r.brandID = scdField(prev.brandID, brandID).(int64)
	fieldChangeFlags >>= 1

	// always uses a new value due to a bug in the C code
	hasSize := atoiOrPanic(itemCategoriesDist.ValueAtIndex(2, categoryIndex))
	sizeWeight := sizeWeightsSized
	if hasSize == 0 {
		sizeWeight = sizeWeightsNoSize
	}
	r.size = itemSizesDist.PickRandomValue(0, sizeWeight, ss.at(iSize))
	fieldChangeFlags >>= 1

	manufactMin, manufactMax := pickItemIDRange(itemManufactIDDist, ss.at(iManufactID))
	manufactID := int64(GenerateUniformRandomInt(manufactMin, manufactMax, ss.at(iManufactID)))
	r.manufactID = scdField(prev.manufactID, manufactID).(int64)
	fieldChangeFlags >>= 1

	manufact := generateWord(r.manufactID, itemRowSizeManufact, syllablesDist)
	r.manufact = scdField(prev.manufact, manufact).(string)
	fieldChangeFlags >>= 1

	formulation := generateRandomCharsetDigits(itemRowSizeFormulation, ss.at(iFormulation))
	formColor := itemColorsDist.PickRandomValue(0, colorsWeightsSkewed, ss.at(iFormulation))
	position := GenerateUniformRandomInt(0, len(formulation)-len(formColor)-1, ss.at(iFormulation))
	formulation = formulation[:position] + formColor + formulation[position+len(formColor):]
	r.formulation = scdField(prev.formulation, formulation).(string)

	// these fields always use a new value due to a bug in the C code
	r.color = itemColorsDist.PickRandomValue(0, colorsWeightsSkewed, ss.at(iColor))
	r.units = itemUnitsDist.PickRandomValue(0, 0, ss.at(iUnits))
	r.container = "Unknown"
	r.productName = generateWord(rowNumber, itemRowSizeProductName, syllablesDist)

	r.promoSk = GenerateJoinKey(TItem, JCNone, ss.at(iPromoSk), TPromotion, 1, sc)
	temp := GenerateUniformRandomInt(1, 100, ss.at(iPromoSk))
	if temp > itemPromoPercentage {
		r.promoSk = -1
	}

	return r
}

// pickCurrentPriceRange draws a [min,max] decimal price range from
// item_current_price.dst using weight column 0. Mirrors pickRandomCurrentPriceRange.
func pickCurrentPriceRange(s *RNStream) (Decimal, Decimal) {
	idx := itemCurrentPriceDist.PickRandomIndex(0, s)

	return ParseDecimal(itemCurrentPriceDist.ValueAtIndex(1, idx)), ParseDecimal(itemCurrentPriceDist.ValueAtIndex(2, idx))
}

// generateRandomCharsetDigits builds a fixed-length string of decimal digits,
// reproducing generateRandomCharset(DIGITS, n, n): it always draws n times
// (min==max==n) and keeps every drawn character. Mirrors generateRandomCharset.
func generateRandomCharsetDigits(n int, s *RNStream) string {
	const digits = "0123456789"
	length := GenerateUniformRandomInt(n, n, s)
	buf := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		idx := GenerateUniformRandomInt(0, len(digits)-1, s)
		if i < length {
			buf = append(buf, digits[idx])
		}
	}

	return string(buf)
}

// Item is the TPC-DS item table. It keeps history (SCD). Mirrors
// ItemRowGenerator's draw order, SCD field-change logic, and ItemRow.getValues
// output order/formatting.
var Item = &Table{
	Name: "item",
	ID:   TItem,
	Columns: []string{
		"i_item_sk", "i_item_id", "i_rec_start_date", "i_rec_end_date",
		"i_item_desc", "i_current_price", "i_wholesale_cost", "i_brand_id",
		"i_brand", "i_class_id", "i_class", "i_category_id", "i_category",
		"i_manufact_id", "i_manufact", "i_size", "i_formulation", "i_color",
		"i_units", "i_container", "i_manager_id", "i_product_name",
	},
	Cols:     itemCols,
	RowCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TItem) },
	Row: func(rowNumber int64, ss *streamSet, sc *Scaling) []any {
		r := computeItem(rowNumber, ss, sc)
		nb := r.nullBitMap

		// Output in ItemRow.getValues order; a nulled column becomes nil (empty
		// field). Key/date columns are nil when their sentinel (-1) is set.
		vals := make([]any, 22)
		if !iIsNull(nb, iItemSk) && rowNumber != -1 {
			vals[0] = rowNumber
		}
		if !iIsNull(nb, iItemID) {
			vals[1] = r.itemID
		}
		if !iIsNull(nb, iRecStartDateID) && r.recStart >= 0 {
			vals[2] = FromJulianDays(int(r.recStart))
		}
		if !iIsNull(nb, iRecEndDateID) && r.recEnd >= 0 {
			vals[3] = FromJulianDays(int(r.recEnd))
		}
		if !iIsNull(nb, iItemDesc) {
			vals[4] = r.desc
		}
		if !iIsNull(nb, iCurrentPrice) {
			vals[5] = r.currentPrice
		}
		if !iIsNull(nb, iWholesaleCost) {
			vals[6] = r.wholesale
		}
		if !iIsNull(nb, iBrandID) && r.brandID != -1 {
			vals[7] = r.brandID
		}
		if !iIsNull(nb, iBrand) {
			vals[8] = r.brand
		}
		if !iIsNull(nb, iClassID) && r.classID != -1 {
			vals[9] = r.classID
		}
		if !iIsNull(nb, iClass) {
			vals[10] = r.class
		}
		if !iIsNull(nb, iCategoryID) && r.categoryID != -1 {
			vals[11] = r.categoryID
		}
		if !iIsNull(nb, iCategory) {
			vals[12] = r.category
		}
		if !iIsNull(nb, iManufactID) && r.manufactID != -1 {
			vals[13] = r.manufactID
		}
		if !iIsNull(nb, iManufact) {
			vals[14] = r.manufact
		}
		if !iIsNull(nb, iSize) {
			vals[15] = r.size
		}
		if !iIsNull(nb, iFormulation) {
			vals[16] = r.formulation
		}
		if !iIsNull(nb, iColor) {
			vals[17] = r.color
		}
		if !iIsNull(nb, iUnits) {
			vals[18] = r.units
		}
		if !iIsNull(nb, iContainer) {
			vals[19] = r.container
		}
		if !iIsNull(nb, iManagerID) && r.managerID != -1 {
			vals[20] = r.managerID
		}
		if !iIsNull(nb, iProductName) {
			vals[21] = r.productName
		}

		return vals
	},
}
