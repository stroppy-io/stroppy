package dsdgen

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var errRefreshInput = errors.New("dsdgen: refresh requires finite scale in (0,100000] and positive set")

// RefreshFiles uses the operational source schema, not warehouse fact rows.
var RefreshFiles = []string{
	"s_purchase", "s_purchase_lineitem", "s_store_returns",
	"s_catalog_order", "s_catalog_order_lineitem", "s_catalog_returns",
	"s_web_order", "s_web_order_lineitem", "s_web_returns", "s_inventory",
	"delete", "inventory_delete",
}

type refreshWriter struct {
	ctx     context.Context
	files   map[string]*os.File
	writers map[string]*bufio.Writer
	err     error
}

// GenerateRefresh writes one numbered source-schema set with the same RNG and
// distributions as the initial load. Generation is outside maintenance timing.
func GenerateRefresh(ctx context.Context, scale float64, set int64, directory string) (err error) {
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 || scale > 100000 || set < 1 {
		return errRefreshInput
	}
	out := &refreshWriter{ctx: ctx, files: map[string]*os.File{}, writers: map[string]*bufio.Writer{}}
	defer func() {
		for name, file := range out.files {
			err = errors.Join(err, out.writers[name].Flush(), file.Close())
		}
	}()
	for _, name := range RefreshFiles {
		file, openErr := os.OpenFile(filepath.Join(directory, name+".dat"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if openErr != nil {
			return openErr
		}
		out.files[name], out.writers[name] = file, bufio.NewWriter(file)
	}
	salesDates, inventoryDates, err := refreshDates(ctx, set)
	if err != nil {
		return err
	}
	for i := 0; i < 6; i += 2 {
		out.row("delete", refreshDate(salesDates[i]), refreshDate(salesDates[i+1]))
		out.row("inventory_delete", refreshDate(inventoryDates[i]), refreshDate(inventoryDates[i+1]))
	}
	scaling := NewScaling(scale)
	// Kit 4.0.0 getUpdateBase indexes the wrong table; reserve keys after earlier sets.
	bases := map[TableID]int64{
		TStoreSales:   scaling.RowCount(TStoreSales),
		TCatalogSales: scaling.RowCount(TCatalogSales),
		TWebSales:     scaling.RowCount(TWebSales),
	}
	for prior := int64(1); prior < set; prior++ {
		dates, _, dateErr := refreshDates(ctx, prior)
		if dateErr != nil {
			return dateErr
		}
		for table := range bases {
			for _, date := range dates {
				bases[table] += scaling.RowCountForDate(table, date)
			}
		}
	}
	for _, channel := range []string{"store", "catalog", "web"} {
		generateRefreshChannel(out, scaling, channel, salesDates, bases)
	}
	quantity := NewRNStream(624, 1)
	items, warehouses := scaling.IDCount(TItem), scaling.RowCount(TWarehouse)
	for _, date := range inventoryDates {
		for warehouse := int64(1); warehouse <= warehouses; warehouse++ {
			for item := int64(1); item <= items; item++ {
				out.row("s_inventory", MakeBusinessKey(warehouse), MakeBusinessKey(firstRefreshSK(item)),
					refreshDate(date), GenerateUniformRandomInt(0, 250, quantity))
				if out.err != nil {
					return out.err
				}
			}
		}
	}
	return out.err
}

func (w *refreshWriter) row(table string, values ...any) {
	if w.err != nil {
		return
	}
	if w.err = w.ctx.Err(); w.err != nil {
		return
	}
	var text strings.Builder
	for _, value := range values {
		if value != nil {
			fmt.Fprint(&text, value)
		}
		text.WriteByte('|')
	}
	text.WriteByte('\n')
	_, w.err = w.writers[table].WriteString(text.String())
}

func refreshDate(julian int64) string {
	if julian < 0 {
		return ""
	}
	date := FromJulianDays(int(julian))
	return fmt.Sprintf("%04d-%02d-%02d", date.Year, date.Month, date.Day)
}

func refreshDates(ctx context.Context, set int64) (sales, inventory [6]int64, err error) {
	random := NewRNStream(0, 0)
	for range set {
		if err = ctx.Err(); err != nil {
			return sales, inventory, err
		}
		for i, weight := range []CalendarWeights{CalLow, CalMedium, CalHigh} {
			index := calendarDist.PickRandomIndex(int(weight), random)
			year := GenerateUniformRandomInt(DateMinimum.Year, DateMaximum.Year, random)
			day, _ := strconv.Atoi(calendarDist.ValueAtIndex(2, index))
			month, _ := strconv.Atoi(calendarDist.ValueAtIndex(4, index))
			date := Date{year, month, day}
			julian := int64(ToJulianDays(date))
			sales[2*i], sales[2*i+1] = julian, julian+1
			if CalendarWeightForDayNumber(CalendarIndexForDate(date)+1, weight) == 0 {
				sales[2*i+1] = julian - 1
			}
			thursday := julian + int64(4-ComputeDayOfWeek(date))
			if CalendarWeightForDayNumber(CalendarIndexForDate(FromJulianDays(int(thursday))), weight) == 0 {
				thursday -= 7
				if CalendarWeightForDayNumber(CalendarIndexForDate(FromJulianDays(int(thursday))), weight) == 0 {
					thursday += 14
				}
			}
			inventory[2*i], inventory[2*i+1] = thursday, thursday+7
			if CalendarWeightForDayNumber(CalendarIndexForDate(FromJulianDays(int(thursday+7)))+1, weight) == 0 {
				inventory[2*i+1] -= 14
			}
		}
	}
	return sales, inventory, nil
}

func firstRefreshSK(id int64) int64 {
	switch id % 3 {
	case 1:
		return id/3*6 + 1
	case 2:
		return id/3*6 + 2
	default:
		return id/3*6 - 2
	}
}

type refreshRandom map[int]*RNStream

func (r refreshRandom) at(column int) *RNStream {
	if r[column] == nil {
		r[column] = NewRNStream(column, 0)
	}
	return r[column]
}

func (r refreshRandom) finish(first int, budgets []int) {
	for offset, budget := range budgets {
		stream := r.at(first + offset)
		for stream.SeedsUsed() < budget {
			stream.NextRandom()
		}
		stream.ResetSeedsUsed()
	}
}

func refreshTime(value int64) string {
	return fmt.Sprintf("%02d:%02d:%02d", value/3600, value%3600/60, value%60)
}

func generateRefreshChannel(out *refreshWriter, scaling *Scaling, channel string, dates [6]int64, bases map[TableID]int64) {
	random := refreshRandom{}
	lines, itemColumn, permutationColumn, table := 12, 679, 689, TStoreSales
	if channel == "catalog" {
		lines, itemColumn, permutationColumn, table = 9, 524, 538, TCatalogSales
	}
	if channel == "web" {
		itemColumn, permutationColumn, table = 748, 760, TWebSales
	}
	var permutation []int
	join := func(column int, target TableID, count int64) int64 {
		return GenerateJoinKey(TDbgenVersion, JCNone, random.at(column), target, count, scaling)
	}
	order := bases[table]
	for _, date := range dates {
		for range scaling.RowCountForDate(table, date) {
			if out.err != nil {
				return
			}
			order++
			var bill, ship, store, site int64
			timeValue := 0
			switch channel {
			case "store":
				store, bill = join(671, TStore, 1), join(672, TCustomer, 1)
				register, clerk := GenerateUniformRandomInt(1, 17, random.at(675)), GenerateUniformRandomInt(101, 300, random.at(676))
				out.row("s_purchase", order, MakeBusinessKey(store), MakeBusinessKey(bill), refreshDate(date), 0, register, clerk, GenerateRandomText(60, 100, random.at(677)))
			case "catalog", "web":
				base := 518
				if channel == "web" {
					base = 741
				}
				timeValue = GenerateUniformRandomInt(0, 86399, random.at(base+3))
				bill = join(base, TCustomer, 1)
				ship = bill
				if GenerateUniformRandomInt(0, 99, random.at(base+1)) > 7 {
					ship = join(base+1, TCustomer, 2)
				}
				mode := join(base+4, TShipMode, 1)
				name, commentColumn := "s_catalog_order", 525
				if channel == "web" {
					site, name, commentColumn = join(746, TWebSite, 1), "s_web_order", 747
				}
				out.row(name, order, MakeBusinessKey(bill), MakeBusinessKey(ship), refreshDate(date), timeValue, MakeBusinessKey(mode), MakeBusinessKey(site), GenerateRandomText(60, 100, random.at(commentColumn)))
			}
			itemIndex := GenerateUniformRandomInt(1, int(scaling.IDCount(TItem)), random.at(itemColumn))
			if permutation == nil {
				permutation = MakePermutation(int(scaling.IDCount(TItem)), random.at(permutationColumn))
			}
			for line := 1; line <= lines; line++ {
				itemIndex = itemIndex%len(permutation) + 1
				item := MakeBusinessKey(firstRefreshSK(int64(PermutationEntry(permutation, itemIndex))))
				base, pricingColumn := 682, 687
				if channel == "catalog" {
					base, pricingColumn = 528, 535
				}
				if channel == "web" {
					base, pricingColumn = 751, 757
				}
				promo := join(base, TPromotion, 1)
				var warehouse, shipDate, page int64
				if channel != "store" {
					warehouse = join(base+3, TWarehouse, 1)
					shipDate = date + int64(GenerateUniformRandomInt(0, 60, random.at(base+4)))
					if shipDate > int64(JulianTodaysDate) {
						shipDate = -1
					}
					if channel == "catalog" {
						page = join(534, TCatalogPage, date)
					} else {
						page = join(756, TWebPage, 1)
					}
				} else {
					GenerateUniformRandomInt(1, 100, random.at(684))
				}
				pricing := GeneratePricingForSales(WebPricingLimits, random.at(pricingColumn))
				switch channel {
				case "store":
					out.row("s_purchase_lineitem", order, line, item, MakeBusinessKey(promo), pricing.Quantity, pricing.SalesPrice, pricing.CouponAmount, GenerateRandomText(1, 100, random.at(686)))
				case "catalog":
					pagesPerCatalog := scaling.RowCount(TCatalogPage) / 108
					out.row("s_catalog_order_lineitem", order, line, item, MakeBusinessKey(promo), pricing.Quantity, pricing.SalesPrice, pricing.CouponAmount, MakeBusinessKey(warehouse), refreshDate(shipDate), page/pagesPerCatalog, page, pricing.ShipCost)
				case "web":
					out.row("s_web_order_lineitem", order, line, item, MakeBusinessKey(promo), pricing.Quantity, pricing.SalesPrice, pricing.CouponAmount, MakeBusinessKey(warehouse), refreshDate(shipDate), pricing.ShipCost, MakeBusinessKey(page))
				}
				returnColumn, returnMaximum, returnThreshold := 688, 99, 10
				if channel == "catalog" {
					returnColumn, returnMaximum, returnThreshold = 537, 9999, 1030
				}
				if channel == "web" {
					returnColumn = 759
				}
				returned := GenerateUniformRandomInt(0, returnMaximum, random.at(returnColumn)) < returnThreshold
				if channel == "catalog" {
					returned = returned && shipDate >= 0
				}
				if returned {
					generateRefreshReturn(out, random, join, channel, order, line, item, bill, ship, store, site, page, date, shipDate, pricing)
				}
			}
			switch channel {
			case "store":
				random.finish(670, []int{0, 1, 1, 1, 1, 1, 1, 100, 7, 1})
				random.finish(680, []int{12, 12, 12, 12, 12, 12, 1200, 96, 12, 0})
				random.finish(710, []int{0, 0, 0, 0, 0, 24, 12, 0, 0, 0, 0, 0, 0, 0, 0, 0, 12, 84})
			case "catalog":
				random.finish(517, []int{0, 1, 2, 1, 1, 1, 1, 1, 100})
				random.finish(526, []int{1, 0, 1, 1, 1, 1, 1, 1, 2, 8, 0, 1, 0})
				random.finish(551, []int{9, 0, 0, 0, 0, 0, 9, 18, 0, 0, 0, 0, 0, 0, 0, 0, 9, 72, 9, 9, 0})
			case "web":
				random.finish(740, []int{1, 1, 2, 2, 1, 1, 1, 100, 1})
				random.finish(749, []int{1, 1, 1, 1, 1, 1, 1, 1, 8, 0, 1, 0})
				random.finish(778, []int{0, 0, 0, 0, 0, 0, 24, 12, 12, 84})
			}
		}
	}
}

func generateRefreshReturn(out *refreshWriter, random refreshRandom, join func(int, TableID, int64) int64,
	channel string, order int64, line int, item string, bill, ship, store, site, page, date, shipDate int64, pricing Pricing) {
	dateColumn, timeColumn, reasonColumn, pricingColumn := 715, 716, 726, 727
	if channel == "catalog" {
		dateColumn, timeColumn, reasonColumn, pricingColumn = 557, 558, 567, 568
	}
	if channel == "web" {
		dateColumn, timeColumn, reasonColumn, pricingColumn = 784, 785, 786, 787
	}
	minLag := 0
	if channel == "web" {
		minLag = 1
	}
	returnedDate := date + int64(GenerateUniformRandomInt(minLag, 60, random.at(dateColumn)))
	if channel == "catalog" {
		returnedDate += shipDate - date
	}
	timeValue := join(timeColumn, TTimeDim, 1)
	callCenter := int64(0)
	if channel == "catalog" {
		callCenter = join(551, TCallCenter, 1)
	}
	reason := join(reasonColumn, TReason, 1)
	quantity := GenerateUniformRandomInt(1, pricing.Quantity, random.at(pricingColumn))
	// Source-return pricing retains the original sale's amounts in kit 4.0.0.
	if channel == "store" {
		out.row("s_store_returns", MakeBusinessKey(store), order, line, item, MakeBusinessKey(bill), refreshDate(returnedDate), refreshTime(timeValue), order, quantity, pricing.SalesPrice, pricing.ExtTax, pricing.Fee, pricing.ExtShipCost, pricing.RefundedCash, pricing.ReversedCharge, pricing.StoreCredit, MakeBusinessKey(reason))
	} else if channel == "catalog" {
		mode, warehouse := join(569, TShipMode, 1), join(570, TWarehouse, 1)
		out.row("s_catalog_returns", MakeBusinessKey(callCenter), order, line, item, MakeBusinessKey(ship), MakeBusinessKey(ship), refreshDate(returnedDate), refreshTime(timeValue), quantity, pricing.NetPaid, pricing.ExtTax, pricing.Fee, pricing.ExtShipCost, pricing.RefundedCash, pricing.ReversedCharge, pricing.StoreCredit, MakeBusinessKey(reason), MakeBusinessKey(mode), MakeBusinessKey(warehouse), MakeBusinessKey(page))
	} else {
		reason = join(reasonColumn, TReason, 1)
		out.row("s_web_returns", MakeBusinessKey(site), order, line, item, MakeBusinessKey(ship), MakeBusinessKey(ship), refreshDate(returnedDate), refreshTime(timeValue), quantity, pricing.ExtSalesPrice, pricing.ExtTax, pricing.Fee, pricing.ExtShipCost, pricing.RefundedCash, pricing.ReversedCharge, pricing.StoreCredit, MakeBusinessKey(reason))
	}
}
