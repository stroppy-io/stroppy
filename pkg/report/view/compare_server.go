package view

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/pkg/report/compare"
)

type catalogSelection struct {
	ID     string
	OnPage bool
}

func selectedReportIDs(query url.Values) ([]string, error) {
	ids := []string{}
	for _, id := range query["id"] {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}

	if len(ids) > compare.MaxRuns {
		return nil, compare.ErrSelection
	}

	return ids, nil
}

func serveComparison(w http.ResponseWriter, r *http.Request, history *report.History, export bool) {
	ids, err := selectedReportIDs(r.URL.Query())
	if err != nil {
		reportHTTPError(w, err)

		return
	}

	result, err := compare.Load(history, ids, r.URL.Query().Get("reference"))
	if err != nil {
		if errors.Is(err, compare.ErrSelection) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			reportHTTPError(w, err)
		}

		return
	}

	var buffer bytes.Buffer
	if err := RenderComparison(&buffer, result, history.Directory, r.URL.Query(), !export); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if export {
		w.Header().Set("Content-Disposition", `attachment; filename="comparison.html"`)
	}

	_, _ = w.Write(buffer.Bytes())
}

func prepareCatalogSelection(page *catalogPage, history *report.History) {
	seen := map[string]bool{}
	retained := []string{}

	for _, id := range page.Query["id"] {
		if seen[id] {
			continue
		}

		seen[id] = true
		if _, err := history.Get(id); err != nil {
			page.SelectionProblems = append(page.SelectionProblems, "Dropped selection "+id+": "+err.Error())

			continue
		}

		retained = append(retained, id)
		onPage := slices.ContainsFunc(page.Entries, func(entry report.HistoryEntry) bool { return entry.ID == id })
		page.Selection = append(page.Selection, catalogSelection{ID: id, OnPage: onPage})
	}

	page.Query["id"] = retained
	if len(retained) > compare.MaxRuns {
		page.SelectionProblems = append(page.SelectionProblems,
			"More than eight reports selected; uncheck reports before comparing.")
	}

	page.Page, _ = strconv.Atoi(page.Query.Get("page"))
}
