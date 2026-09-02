package baseline

import (
	"fmt"
	"io"
)

const (
	thousand = 1_000
	million  = 1_000_000

	diffSignificantPct = 2.0
)

// tierTitle describes what one tier measures.
func tierTitle(name string, report *Report) string {
	switch name {
	case tierNoop:
		return "noop — framework ceiling (noop driver)"
	case tierWire:
		return fmt.Sprintf("wire — protocol ceiling (pg wire via pg-noop %s, loopback)", report.PGNoop)
	default:
		return name
	}
}

func renderText(out io.Writer, report *Report) {
	host := report.Host
	fmt.Fprintf(out, "stroppy baseline %s — %s/%s, %d cpus, %s\n\n",
		report.Stroppy, host.OS, host.Arch, host.MaxProcs, host.GoVersion)

	for idx := range report.Tiers {
		tier := &report.Tiers[idx]

		fmt.Fprintf(out, "%s\n", tierTitle(tier.Name, report))
		fmt.Fprintf(out, "  load      %-16s %s rows/s\n",
			fmt.Sprintf("%.0f rows", tier.Load.Rows), formatRate(tier.Load.RowsPerSec))
		fmt.Fprintf(out, "  tx 1 VU   %-16s avg %.3fms\n",
			formatRate(tier.TxSingle.TxPerSec)+" tx/s", tier.TxSingle.AvgMs)
		fmt.Fprintf(out, "  tx %d VU  %-16s p50 %.3fms  p99 %.3fms\n\n",
			tier.ParallelVUs,
			formatRate(tier.TxParallel.TxPerSec)+" tx/s",
			tier.TxParallel.P50Ms, tier.TxParallel.P99Ms)
	}

	ok, warn := countVerdicts(report.Verdicts)

	fmt.Fprintf(out, "verdicts: %d ok, %d warn\n", ok, warn)

	for idx := range report.Verdicts {
		verdict := &report.Verdicts[idx]
		fmt.Fprintf(out, "  %-4s %s: %s\n", verdict.Status, verdict.Check, verdict.Detail)
	}
}

func countVerdicts(verdicts []Verdict) (ok, warn int) {
	for idx := range verdicts {
		if verdicts[idx].Status == statusWarn {
			warn++
		} else {
			ok++
		}
	}

	return ok, warn
}

type diffEntry struct {
	label    string
	current  float64
	previous float64
}

func renderDiff(out io.Writer, previous, current *Report) {
	prevTiers := make(map[string]TierResult, len(previous.Tiers))
	for idx := range previous.Tiers {
		prevTiers[previous.Tiers[idx].Name] = previous.Tiers[idx]
	}

	var deltas []diffEntry

	for idx := range current.Tiers {
		tier := &current.Tiers[idx]

		prev, found := prevTiers[tier.Name]
		if !found {
			continue
		}

		// Load throughput only compares at equal row counts: generation cost
		// per row differs between a small and a large load.
		if tier.Load.Rows == prev.Load.Rows && prev.Load.RowsPerSec > 0 {
			deltas = append(deltas, diffEntry{
				label: tier.Name + " load", current: tier.Load.RowsPerSec, previous: prev.Load.RowsPerSec,
			})
		}

		deltas = append(deltas, diffEntry{
			label: tier.Name + " tx 1VU", current: tier.TxSingle.TxPerSec, previous: prev.TxSingle.TxPerSec,
		})

		// Parallel phases only compare at equal VU counts: different
		// concurrency changes the number, not the machine.
		if tier.ParallelVUs == prev.ParallelVUs {
			deltas = append(deltas, diffEntry{
				label:    fmt.Sprintf("%s tx %dVU", tier.Name, tier.ParallelVUs),
				current:  tier.TxParallel.TxPerSec,
				previous: prev.TxParallel.TxPerSec,
			})

			if tier.Name == tierWire {
				deltas = append(deltas, diffEntry{
					label: "wire p99", current: tier.TxParallel.P99Ms, previous: prev.TxParallel.P99Ms,
				})
			}
		}
	}

	if len(deltas) == 0 {
		return
	}

	fmt.Fprintf(out, "vs %s:\n", previous.Time.Format("2006-01-02 15:04"))

	for idx := range deltas {
		d := &deltas[idx]
		if d.previous <= 0 {
			continue
		}

		change := (d.current - d.previous) / d.previous * percentScale

		marker := "="
		if change > diffSignificantPct {
			marker = "+"
		}

		if change < -diffSignificantPct {
			marker = "-"
		}

		fmt.Fprintf(out, "  %s %-14s %+6.1f%%\n", marker, d.label, change)
	}
}

// formatRate renders a per-second rate with k/M suffixes.
func formatRate(value float64) string {
	switch {
	case value >= million:
		return fmt.Sprintf("%.2fM", value/million)
	case value >= thousand:
		return fmt.Sprintf("%.1fk", value/thousand)
	default:
		return fmt.Sprintf("%.0f", value)
	}
}
