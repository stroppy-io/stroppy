package view

import (
	"encoding/json"
	"io"
	"net/url"
	"slices"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func catalogOptions(entries []report.HistoryEntry, query url.Values) (workloads, drivers []string) {
	workloadNames, driverNames := map[string]any{}, map[string]any{}
	for i := range entries {
		entry := &entries[i]
		if entry.Workload != "" {
			workloadNames[entry.Workload] = nil
		}
		if entry.Driver != "" {
			driverNames[entry.Driver] = nil
		}
		for _, driver := range entry.Drivers {
			if driver.Type != "" {
				driverNames[driver.Type] = nil
			}
		}
	}
	for _, value := range query["workload"] {
		if value != "" {
			workloadNames[value] = nil
		}
	}
	for _, value := range query["driver"] {
		if value != "" {
			driverNames[value] = nil
		}
	}

	return sortedKeys(workloadNames), sortedKeys(driverNames)
}

func filterCatalogCapture(history *report.History, entries []report.HistoryEntry, query url.Values) []report.HistoryEntry {
	metrics, logs := query.Get("metrics") == "complete", query.Get("logs") == "complete"
	if !metrics && !logs {
		return entries
	}

	return slices.DeleteFunc(entries, func(entry report.HistoryEntry) bool {
		data, err := history.Get(entry.ID)
		if err != nil {
			return true
		}
		capture, err := report.CaptureFromJSON(data)
		if err != nil || capture == nil {
			return true
		}
		file, err := report.OpenCaptureFile(history.Directory, capture, "manifest.json")
		if err != nil {
			return true
		}
		var manifest report.Capture
		err = json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&manifest)
		file.Close()
		if err != nil || manifest.Schema != report.CaptureSchema || manifest.ID != capture.ID ||
			manifest.FinishedAt.IsZero() || (manifest.State != "complete" && manifest.State != "incomplete") {
			return true
		}

		capture.Streams = manifest.Streams
		return (metrics && (manifest.MissedSamples > 0 || capture.Streams["metrics"].Records == 0 ||
			!completeCatalogStream(history.Directory, capture, "metrics"))) ||
			(logs && !completeCatalogStream(history.Directory, capture, "logs"))
	})
}

func completeCatalogStream(base string, capture *report.Capture, name string) bool {
	stream := capture.Streams[name]
	if !stream.Enabled || stream.Dropped > 0 || stream.Truncated > 0 || stream.Error != "" {
		return false
	}
	file, err := report.OpenCaptureFile(base, capture, name+".jsonl")
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()

	return err == nil && info.Size() == stream.Bytes
}
