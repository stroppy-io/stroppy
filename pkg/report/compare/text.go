package compare

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// WriteText renders the same values and reasons exposed by Result JSON.
func WriteText(output io.Writer, result *Result) error {
	const (
		tabWidth = 4
		padding  = 2
	)

	writer := tabwriter.NewWriter(output, 0, tabWidth, padding, ' ', 0)
	fmt.Fprintf(writer, "Reference: %s\n", result.Reference)

	for _, run := range result.Runs {
		fmt.Fprintf(writer, "%s\t%s\t%s\n", run.ID, run.Workload, run.Status)

		for _, warning := range run.Warnings {
			fmt.Fprintf(writer, "  warning: %s\n", warning)
		}
	}

	for _, warning := range result.Warnings {
		fmt.Fprintf(writer, "Warning: %s\n", warning)
	}

	fmt.Fprintln(writer, "\nConfiguration and result differences")

	for _, difference := range result.Differences {
		fmt.Fprint(writer, difference.Path)

		for _, value := range difference.Values {
			text := "unavailable"
			if value.Present {
				text = encoded(value.Data)
			}

			fmt.Fprintf(writer, "\t%s", text)
		}

		fmt.Fprintln(writer)
	}

	fmt.Fprintln(writer, "\nMetric\tScope\tStatistic\tRun\tValue\tChange\tpercent")

	for index := range result.Metrics {
		row := &result.Metrics[index]
		for i := range row.Cells {
			value, delta, percent := cellText(&row.Cells[i])

			statistic := row.Statistic
			if row.Approximate {
				statistic += " (approximate histogram estimate)"
			}

			fmt.Fprintf(writer, "%s [%s]\t%s %s\t%s\t%s\t%s\t%s\t%s\n",
				row.Name, row.ValueUnit, row.Scope, encoded(row.Attributes), statistic, result.Runs[i].ID,
				strings.ReplaceAll(value, "\n", " "), delta, percent)
		}
	}

	return writer.Flush()
}

func cellText(cell *Cell) (value, delta, percent string) {
	value, delta, percent = "unavailable: "+cell.Reason, "—", "—"
	if cell.Value != nil {
		value = string(*cell.Value)
	}

	if cell.Delta != nil {
		delta = string(*cell.Delta)
	} else if cell.DeltaReason != "" {
		delta = cell.DeltaReason
	}

	if cell.Percent != nil {
		percent = string(*cell.Percent)
	} else if cell.PercentReason != "" {
		percent = cell.PercentReason
	}

	if cell.Rounded {
		percent = "≈" + percent
	}

	return value, delta, percent
}
