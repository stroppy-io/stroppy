package view

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestCatalogRetainsOversizedSelectionAndDropsStaleIDs(t *testing.T) {
	t.Parallel()
	history := &report.History{Directory: t.TempDir()}
	ids := make([]string, 0, 9)

	for i := range 9 {
		path, err := history.Save(&report.Run{
			Schema: report.SchemaVersion, Kind: report.Kind, ID: strconv.Itoa(i), Workload: "selection",
			Status: report.StatusCompleted, StartedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}

		ids = append(ids, strings.TrimSuffix(filepath.Base(path), ".json"))
	}

	stale := "2026-10-15T00-00-00Z-removed"
	query := url.Values{"id": append(append([]string{}, ids...), stale, "invalid", ids[0])}
	request := httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil)
	response := httptest.NewRecorder()
	Handler(history).ServeHTTP(response, request)

	page := response.Body.String()
	if response.Code != 200 || !strings.Contains(page, "More than eight") ||
		!strings.Contains(page, "Dropped selection "+stale) {
		t.Fatalf("catalog selection blocked browsing: %d %.200s", response.Code, page)
	}

	if strings.Count(page, ` checked>`) != len(ids) || strings.Contains(page, `value="`+stale+`"`) {
		t.Fatal("valid selections lost or stale selection resubmitted")
	}

	comparison := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/compare?"+url.Values{"id": ids}.Encode(), nil)
	Handler(history).ServeHTTP(comparison, request)

	if comparison.Code < 400 {
		t.Fatal("oversized comparison accepted")
	}
}
