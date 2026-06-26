package dsdgen

// Inventory column stream layout. Global numbers/seeds from
// InventoryGeneratorColumn.java. Only INV_NULLS and INV_QUANTITY_ON_HAND draw;
// the key columns are derived from the row number.
const (
	invDateSk = iota
	invItemSk
	invWarehouseSk
	invQuantityOnHand
	invNulls
)

var inventoryCols = []GeneratorColumn{
	invDateSk:         {GlobalColumnNumber: 198, SeedsPerRow: 1},
	invItemSk:         {GlobalColumnNumber: 199, SeedsPerRow: 1},
	invWarehouseSk:    {GlobalColumnNumber: 200, SeedsPerRow: 1},
	invQuantityOnHand: {GlobalColumnNumber: 201, SeedsPerRow: 1},
	invNulls:          {GlobalColumnNumber: 202, SeedsPerRow: 2},
}

const (
	inventoryNullBasis     = 1000
	inventoryNotNullBitMap = 0x07
	invFirstColumnGlobal   = 198
)

func invIsNull(nullBitMap int64, localIdx int) bool {
	off := inventoryCols[localIdx].GlobalColumnNumber - invFirstColumnGlobal

	return nullBitMap&(int64(1)<<uint(off)) != 0
}

// Inventory is the TPC-DS inventory table: a date-based table whose row number
// decomposes into (item, warehouse, week). Item is a history dimension, so the
// item surrogate key is date-matched. Mirrors InventoryRowGenerator.
var Inventory = &Table{
	Name:     "inventory",
	ID:       TInventory,
	Columns:  []string{"inv_date_sk", "inv_item_sk", "inv_warehouse_sk", "inv_quantity_on_hand"},
	Cols:     inventoryCols,
	RowCount: func(sf float64) int64 { return NewScaling(sf).RowCount(TInventory) },
	Row: func(rowNumber int64, ss *streamSet, sc *Scaling) []any {
		nullBitMap := CreateNullBitMap(inventoryNullBasis, inventoryNotNullBitMap, ss.at(invNulls))

		index := rowNumber - 1
		itemCount := sc.IDCount(TItem)
		itemSk := index%itemCount + 1
		index /= itemCount

		warehouseCount := sc.IDCount(TWarehouse)
		warehouseSk := index%warehouseCount + 1
		index /= warehouseCount

		dateSk := int64(JulianDateMinimum) + index*7 // inventory is updated weekly
		itemSk = MatchSurrogateKey(itemSk, dateSk, TItem, sc)

		qty := GenerateUniformRandomInt(0, 1000, ss.at(invQuantityOnHand))

		vals := []any{dateSk, itemSk, warehouseSk, int64(qty)}
		// inv_date_sk, inv_item_sk, inv_warehouse_sk are key columns (nil on -1).
		for i, key := range []bool{true, true, true, false} {
			if invIsNull(nullBitMap, i) || (key && vals[i] == int64(-1)) {
				vals[i] = nil
			}
		}

		return vals
	},
}
