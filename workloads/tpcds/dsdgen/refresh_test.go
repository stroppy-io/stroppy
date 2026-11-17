package dsdgen

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestRefreshOracle(t *testing.T) {
	directory := os.Getenv("DSDGEN_REFRESH_ORACLE")
	if directory == "" {
		t.Skip("DSDGEN_REFRESH_ORACLE not set")
	}
	set := int64(1)
	if raw := os.Getenv("DSDGEN_REFRESH_SET"); raw != "" {
		if _, err := fmt.Sscan(raw, &set); err != nil {
			t.Fatal(err)
		}
	}
	generated := t.TempDir()
	if err := GenerateRefresh(context.Background(), 1, set, generated); err != nil {
		t.Fatal(err)
	}
	for _, name := range RefreshFiles {
		t.Run(name, func(t *testing.T) {
			actual, err := os.Open(filepath.Join(generated, name+".dat"))
			if err != nil {
				t.Fatal(err)
			}
			defer actual.Close()
			expected, err := os.Open(filepath.Join(directory, fmt.Sprintf("%s_%d.dat", name, set)))
			if err != nil {
				t.Fatal(err)
			}
			defer expected.Close()
			a, b := bufio.NewScanner(actual), bufio.NewScanner(expected)
			var offset int64
			for line := 1; ; line++ {
				x, y := a.Scan(), b.Scan()
				if x != y {
					t.Fatalf("row %d: generated=%v oracle=%v", line, x, y)
				}
				if !x {
					break
				}
				actualText, expectedText := a.Text(), b.Text()
				keyColumn := -1
				switch name {
				case "s_purchase", "s_purchase_lineitem", "s_catalog_order", "s_catalog_order_lineitem", "s_web_order", "s_web_order_lineitem":
					keyColumn = 0
				case "s_store_returns", "s_catalog_returns", "s_web_returns":
					keyColumn = 1
				}
				if keyColumn >= 0 {
					cells, oracle := strings.Split(actualText, "|"), strings.Split(expectedText, "|")
					key, _ := strconv.ParseInt(cells[keyColumn], 10, 64)
					old, _ := strconv.ParseInt(oracle[keyColumn], 10, 64)
					if line == 1 {
						offset = key - old
					}
					if key-old != offset || offset <= 0 {
						t.Fatalf("invalid key offset at row %d: %d", line, key-old)
					}
					cells[keyColumn] = oracle[keyColumn]
					if name == "s_store_returns" {
						cells[7] = oracle[7]
					}
					actualText = strings.Join(cells, "|")
				}
				if actualText != expectedText {
					t.Fatalf("row %d\ngot  %s\nwant %s", line, actualText, expectedText)
				}
			}
			if err := a.Err(); err != nil {
				t.Fatal(err)
			}
			if err := b.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRefreshDates(t *testing.T) {
	sales, inventory, err := refreshDates(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	wantSales := []string{"2000-05-20", "2000-05-21", "1999-09-18", "1999-09-19", "2002-11-12", "2002-11-13"}
	wantInventory := []string{"2000-05-18", "2000-05-25", "1999-09-16", "1999-09-23", "2002-11-14", "2002-11-21"}
	var gotSales, gotInventory []string
	for i, date := range sales {
		gotSales = append(gotSales, refreshDate(date))
		gotInventory = append(gotInventory, refreshDate(inventory[i]))
	}
	if !reflect.DeepEqual(gotSales, wantSales) || !reflect.DeepEqual(gotInventory, wantInventory) {
		t.Fatalf("dates: %v %v", gotSales, gotInventory)
	}
}

func TestRefreshKeyRanges(t *testing.T) {
	scaling := NewScaling(.01)
	previous := map[string]int64{
		"s_purchase":      scaling.RowCount(TStoreSales),
		"s_catalog_order": scaling.RowCount(TCatalogSales),
		"s_web_order":     scaling.RowCount(TWebSales),
	}
	for set := int64(1); set <= 3; set++ {
		directory := t.TempDir()
		if err := GenerateRefresh(t.Context(), .01, set, directory); err != nil {
			t.Fatal(err)
		}
		for name, last := range previous {
			data, err := os.ReadFile(filepath.Join(directory, name+".dat"))
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				key, err := strconv.ParseInt(strings.Split(line, "|")[0], 10, 64)
				if err != nil || key != last+1 {
					t.Fatalf("set %d %s key=%d previous=%d", set, name, key, last)
				}
				last = key
			}
			previous[name] = last
		}
	}
}

func TestRefreshReturnQuantities(t *testing.T) {
	for set := int64(1); set <= 2; set++ {
		directory := t.TempDir()
		if err := GenerateRefresh(t.Context(), 1, set, directory); err != nil {
			t.Fatal(err)
		}
		for _, channel := range []struct{ name, sales, returns string }{
			{"store", "s_purchase_lineitem", "s_store_returns"},
			{"catalog", "s_catalog_order_lineitem", "s_catalog_returns"},
			{"web", "s_web_order_lineitem", "s_web_returns"},
		} {
			read := func(name string) [][]string {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(directory, name+".dat"))
				if err != nil {
					t.Fatal(err)
				}
				var rows [][]string
				for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					rows = append(rows, strings.Split(line, "|"))
				}
				return rows
			}
			sales := map[string]int{}
			for _, row := range read(channel.sales) {
				quantity, err := strconv.Atoi(row[4])
				if err != nil {
					t.Fatal(err)
				}
				sales[row[0]+"/"+row[1]+"/"+row[2]] = quantity
			}
			fullReturns := 0
			for _, row := range read(channel.returns) {
				key := row[1] + "/" + row[2] + "/" + row[3]
				quantity, err := strconv.Atoi(row[8])
				if err != nil || quantity < 1 || quantity > sales[key] {
					t.Fatalf("set %d %s return %s: quantity=%d sale=%d error=%v",
						set, channel.name, key, quantity, sales[key], err)
				}
				if quantity == sales[key] {
					fullReturns++
				}
			}
			if fullReturns == 0 {
				t.Fatalf("set %d %s: no full returns", set, channel.name)
			}
			t.Logf("set %d %s: %d full returns with matching sales", set, channel.name, fullReturns)
		}
	}
}

func TestRefreshInvalidAndCanceled(t *testing.T) {
	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1), 100001} {
		if err := GenerateRefresh(context.Background(), scale, 1, t.TempDir()); !errors.Is(err, errRefreshInput) {
			t.Fatalf("scale %g: %v", scale, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := GenerateRefresh(ctx, 1, 1, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
