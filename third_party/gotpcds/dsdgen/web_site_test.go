package dsdgen

import "testing"

func TestWebSiteByteEqual(t *testing.T) {
	assertTableByteEqual(t, WebSite, 1, 10)
}
