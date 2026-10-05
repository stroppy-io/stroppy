package dsdgen

import "testing"

func TestWebSalesByteEqual(t *testing.T)   { assertFactTableByteEqual(t, WebSales, 1) }
func TestWebReturnsByteEqual(t *testing.T) { assertFactTableByteEqual(t, WebReturns, 1) }
