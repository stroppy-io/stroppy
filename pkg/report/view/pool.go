package view

import (
	"encoding/json"
	"fmt"
)

type poolView struct {
	Step, Execution, Database, Driver, Backend, Scope, Status, Reason string
	Open, InUse, Idle, Maximum, Waits, Seconds, Canceled              string
	Semantics                                                         string
}

func poolText(value any) string {
	if value == nil {
		return ""
	}

	return text(value)
}

func poolViews(value any) []poolView {
	items, _ := value.([]any)

	out := make([]poolView, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}

		v := poolView{
			Step: text(entry["step"]), Execution: poolText(entry["execution"]),
			Database: text(entry["database"]), Driver: text(entry["driver"]),
			Backend: text(entry["backend"]), Scope: text(entry["scope"]),
			Reason: poolText(entry["reason"]), Status: "Unavailable",
		}
		if entry["supported"] == true {
			v.Status = "Supported"
			v.Open, v.InUse, v.Idle = text(entry["open"]), text(entry["in_use"]), text(entry["idle"])

			v.Maximum = text(entry["max_connections"])
			if n, numeric := entry["max_connections"].(json.Number); numeric {
				if n.String() == "0" {
					v.Maximum = "Unlimited"
				} else if limit, err := n.Int64(); err == nil && limit < 0 {
					v.Maximum = "Unknown"
				}
			}

			v.Waits, v.Seconds = text(entry["wait_count"]), text(entry["wait_seconds"])

			v.Canceled = text(entry["canceled_acquires"])
			switch v.Backend {
			case "pgxpool":
				v.Semantics = "Successful empty-pool acquisitions; includes new-connection construction, excludes canceled waits."
			case "database/sql":
				v.Semantics = "Capacity waits at the open-connection limit; " +
					"includes canceled waits, excludes direct connection construction."
			default:
				v.Semantics = fmt.Sprintf("Backend-specific wait counters (%s).", v.Backend)
			}
		}

		out = append(out, v)
	}

	return out
}
