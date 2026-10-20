package view

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

const catalogPageSize = 20

var errCatalogQuery = errors.New("invalid report catalog filter")

type catalogPage struct {
	Entries           []report.HistoryEntry
	Problems          []report.HistoryProblem
	Query             url.Values
	Previous          string
	Next              string
	Total             int
	Selection         []catalogSelection
	SelectionProblems []string
	Page              int
	Workloads         []string
	Drivers           []string
}

// Handler serves a read-only history catalog, HTML reports, and original JSON.
func Handler(history *report.History) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /compare", func(w http.ResponseWriter, r *http.Request) {
		serveComparison(w, r, history, false)
	})
	mux.HandleFunc("GET /compare/export", func(w http.ResponseWriter, r *http.Request) {
		serveComparison(w, r, history, true)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		serveCatalog(w, r, history)
	})
	mux.HandleFunc("GET /reports/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := validateCaptureQuery(r.URL.Query()); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		data, err := history.Get(r.PathValue("id"))
		if err != nil {
			reportHTTPError(w, err)

			return
		}

		var buffer bytes.Buffer

		err = renderServed(&buffer, data, r.PathValue("id"), r.URL.Path+"/json", history.Directory, r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)

			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buffer.Bytes())
	})
	mux.HandleFunc("GET /reports/{id}/json", func(w http.ResponseWriter, r *http.Request) {
		data, err := history.Get(r.PathValue("id"))
		if err != nil {
			reportHTTPError(w, err)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="report.json"`)
		_, _ = w.Write(data) //nolint:gosec // validated JSON attachment, never rendered as HTML.
	})

	mux.HandleFunc("GET /reports/{id}/artifacts/{name}", func(w http.ResponseWriter, r *http.Request) {
		file, err := history.OpenArtifact(r.PathValue("id"), r.PathValue("name"))
		if err != nil {
			reportHTTPError(w, err)

			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("name")+`"`)

		if r.Method == http.MethodHead {
			return
		}

		_, _ = io.Copy(w, file)
	})
	mux.HandleFunc("GET /reports/{id}/bundle", func(w http.ResponseWriter, r *http.Request) {
		data, err := history.Get(r.PathValue("id"))
		if err != nil {
			reportHTTPError(w, err)

			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="report.zip"`)

		if r.Method == http.MethodHead {
			return
		}

		if err := BundleData(w, data, r.PathValue("id"), history.Directory); err != nil {
			panic(http.ErrAbortHandler)
		}
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; "+chartPolicy()+"style-src 'unsafe-inline'; "+
			"img-src data:; font-src data:; form-action 'self'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

func serveCatalog(w http.ResponseWriter, r *http.Request, history *report.History) {
	query, err := catalogQuery(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	entries, problems, err := history.List(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	all := entries
	if len(query.Workloads) > 0 || len(query.Drivers) > 0 || len(query.Statuses) > 0 ||
		!query.Since.IsZero() || !query.Until.IsZero() {
		all, _, err = history.List(report.HistoryQuery{})
		if err != nil {
			reportHTTPError(w, err)

			return
		}
	}

	workloads, drivers := catalogOptions(all, r.URL.Query())
	entries = filterCatalogCapture(history, entries, r.URL.Query())

	index, err := strconv.Atoi(r.URL.Query().Get("page"))
	if r.URL.Query().Get("page") == "" {
		index = 0
		err = nil
	}

	lastPage := max(0, (len(entries)-1)/catalogPageSize)
	if err != nil || index < 0 || index > lastPage {
		http.Error(w, "invalid catalog page", http.StatusBadRequest)

		return
	}

	start := index * catalogPageSize
	end := min(start+catalogPageSize, len(entries))

	page := catalogPage{
		Entries: entries[start:end], Problems: problems, Query: r.URL.Query(), Total: len(entries),
		Workloads: workloads, Drivers: drivers,
	}
	prepareCatalogSelection(&page, history)

	if index > 0 {
		page.Previous = catalogLink(r.URL.Query(), index-1)
	}

	if end < len(entries) {
		page.Next = catalogLink(r.URL.Query(), index+1)
	}

	var buffer bytes.Buffer
	if err := templates.ExecuteTemplate(&buffer, "catalog", page); err != nil {
		http.Error(w, "cannot render report catalog", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buffer.Bytes())
}

func reportHTTPError(w http.ResponseWriter, err error) {
	status := http.StatusUnprocessableEntity
	if errors.Is(err, fs.ErrNotExist) {
		status = http.StatusNotFound
	} else if errors.Is(err, report.ErrInvalidHistoryID) {
		status = http.StatusBadRequest
	}

	http.Error(w, err.Error(), status)
}

func catalogQuery(values url.Values) (report.HistoryQuery, error) {
	if order := values.Get("order"); order != "" && order != "newest" && order != "oldest" {
		return report.HistoryQuery{}, errCatalogQuery
	}

	query := report.HistoryQuery{
		Workloads: values["workload"], Drivers: slices.Clone(values["driver"]), Reverse: values.Get("order") == "oldest",
	}
	for i, driver := range query.Drivers {
		switch driver {
		case "pg":
			query.Drivers[i] = "postgres"
		case "pico":
			query.Drivers[i] = "picodata"
		}
	}

	for _, status := range values["status"] {
		switch report.Status(status) {
		case "":
		case report.StatusCompleted, report.StatusCompletedWithErrors, report.StatusFailed, report.StatusCanceled:
			query.Statuses = append(query.Statuses, report.Status(status))
		default:
			return query, fmt.Errorf("%w: invalid status %q", errCatalogQuery, status)
		}
	}

	if len(query.Workloads) == 1 && query.Workloads[0] == "" {
		query.Workloads = nil
	}

	if len(query.Drivers) == 1 && query.Drivers[0] == "" {
		query.Drivers = nil
	}

	var err error

	query.Since, err = catalogTime(values.Get("since"))
	if err != nil {
		return query, err
	}

	query.Until, err = catalogTime(values.Get("until"))

	return query, err
}

func catalogTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	for _, layout := range []string{time.RFC3339Nano, time.DateOnly} {
		if result, err := time.Parse(layout, value); err == nil {
			return result, nil
		}
	}

	return time.Time{}, fmt.Errorf("%w: invalid date %q", errCatalogQuery, value)
}

func catalogLink(values url.Values, page int) string {
	query := url.Values{}
	for key, value := range values {
		query[key] = value
	}

	query.Set("page", strconv.Itoa(page))

	return "/?" + query.Encode()
}
