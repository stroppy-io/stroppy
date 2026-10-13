package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

const (
	defaultReportLimit  = 20
	reportColumnPadding = 2
)

var (
	errReportTime   = errors.New("expected RFC3339 timestamp or YYYY-MM-DD")
	errReportStatus = errors.New("unknown report status")
	errReportAge    = errors.New("--older-than requires a positive duration")
)

func newReportsCommand() *cobra.Command {
	command := &cobra.Command{Use: "reports", Short: "List, retrieve, and remove saved run reports"}
	command.AddCommand(
		newReportsListCommand(), newReportsGetCommand(), newReportsRemoveCommand(),
		newReportsPruneCommand(), newReportsStatusCommand(),
	)

	return command
}

func newReportsListCommand() *cobra.Command {
	var (
		selection reportSelection
		format    string
	)

	command := &cobra.Command{
		Use: "list", Short: "List saved reports, newest first", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := reportOutputFormat(format); err != nil {
				return err
			}

			query, err := selection.query()
			if err != nil {
				return err
			}

			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			entries, problems, err := history.List(query)
			if err != nil {
				return err
			}

			warnReportProblems(cmd.ErrOrStderr(), problems)

			return writeReportEntries(cmd.OutOrStdout(), format, entries)
		},
	}
	selection.flags(command)
	command.Flags().IntVar(&selection.limit, "limit", defaultReportLimit, "maximum results; 0 returns all")
	command.Flags().BoolVar(&selection.reverse, "reverse", false, "list oldest first")
	reportFormatFlag(command, &format)

	return command
}

func newReportsGetCommand() *cobra.Command {
	return &cobra.Command{
		Use: "get ID", Short: "Write a complete saved report as JSON", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			data, err := history.Get(args[0])
			if err != nil {
				return err
			}

			_, err = cmd.OutOrStdout().Write(data)

			return err
		},
	}
}

func newReportsRemoveCommand() *cobra.Command {
	var (
		yes    bool
		format string
	)

	command := &cobra.Command{
		Use: "remove ID [ID...]", Short: "Preview report removal; --yes deletes selected files",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := reportOutputFormat(format); err != nil {
				return err
			}

			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			ids := slices.Clone(args)
			slices.Sort(ids)
			ids = slices.Compact(ids)

			return removeReports(cmd, history, ids, yes, format)
		},
	}
	command.Flags().BoolVarP(&yes, "yes", "y", false, "delete selected reports instead of previewing")
	reportFormatFlag(command, &format)

	return command
}

func newReportsPruneCommand() *cobra.Command {
	var (
		selection reportSelection
		age       time.Duration
		yes       bool
		format    string
	)

	command := &cobra.Command{
		Use: "prune --older-than DURATION", Short: "Preview old-report cleanup; --yes deletes matches",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := reportOutputFormat(format); err != nil {
				return err
			}

			if age <= 0 {
				return errReportAge
			}

			query, err := selection.query()
			if err != nil {
				return err
			}

			cutoff := time.Now().UTC().Add(-age)
			if query.Until.IsZero() || cutoff.Before(query.Until) {
				query.Until = cutoff
			}

			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			entries, problems, err := history.List(query)
			if err != nil {
				return err
			}

			warnReportProblems(cmd.ErrOrStderr(), problems)

			ids := make([]string, 0, len(entries))
			for i := range entries {
				entry := &entries[i]
				ids = append(ids, entry.ID)
			}

			return removeReports(cmd, history, ids, yes, format)
		},
	}
	selection.flags(command)
	command.Flags().DurationVar(&age, "older-than", 0, "age since run start, for example 720h")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "delete matching reports instead of previewing")
	reportFormatFlag(command, &format)

	return command
}

func newReportsStatusCommand() *cobra.Command {
	var format string

	command := &cobra.Command{
		Use: "status", Short: "Show report storage location and health", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := reportOutputFormat(format); err != nil {
				return err
			}

			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			status, err := history.Status()
			if err != nil {
				return err
			}

			warnReportProblems(cmd.ErrOrStderr(), status.Problems)

			if format == "json" {
				return writeReportJSON(cmd.OutOrStdout(), status)
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"Directory: %s\nExists: %t\nReports: %d\nUnreadable: %d\nBytes: %d\n",
				status.Directory, status.Exists, status.Reports, status.Unreadable, status.SizeBytes,
			)

			return err
		},
	}
	reportFormatFlag(command, &format)

	return command
}

type reportSelection struct {
	workloads []string
	drivers   []string
	statuses  []string
	since     string
	until     string
	limit     int
	reverse   bool
}

func (s *reportSelection) flags(command *cobra.Command) {
	flags := command.Flags()
	flags.StringSliceVar(&s.workloads, "workload", nil, "exact workload names; repeated values match any")
	flags.StringSliceVar(&s.drivers, "driver", nil, "driver types or presets; repeated values match any")
	flags.StringSliceVar(&s.statuses, "status", nil,
		"completed, completed_with_errors, failed, or canceled; repeated values match any")
	flags.StringVar(&s.since, "since", "", "inclusive start time, RFC3339 or YYYY-MM-DD (UTC)")
	flags.StringVar(&s.until, "until", "", "exclusive start time, RFC3339 or YYYY-MM-DD (UTC)")
}

func (s *reportSelection) query() (report.HistoryQuery, error) {
	query := report.HistoryQuery{Workloads: s.workloads, Limit: s.limit, Reverse: s.reverse}
	for _, name := range s.drivers {
		switch name {
		case "pg":
			name = "postgres"
		case "pico":
			name = "picodata"
		}

		query.Drivers = append(query.Drivers, name)
	}

	for _, status := range s.statuses {
		switch report.Status(status) {
		case report.StatusCompleted, report.StatusCompletedWithErrors, report.StatusFailed, report.StatusCanceled:
			query.Statuses = append(query.Statuses, report.Status(status))
		default:
			return query, fmt.Errorf("%w: %q", errReportStatus, status)
		}
	}

	var err error

	query.Since, err = parseReportTime(s.since)
	if err != nil {
		return query, fmt.Errorf("--since: %w", err)
	}

	query.Until, err = parseReportTime(s.until)
	if err != nil {
		return query, fmt.Errorf("--until: %w", err)
	}

	return query, nil
}

func parseReportTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	for _, layout := range []string{time.RFC3339Nano, time.DateOnly} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("%w: %q", errReportTime, value)
}

func reportFormatFlag(command *cobra.Command, format *string) {
	command.Flags().StringVarP(format, "output", "o", "human", "output format: human or json")
}

func reportOutputFormat(format string) error {
	if format != "human" && format != "json" {
		return fmt.Errorf("%w: %q", errOutputFormat, format)
	}

	return nil
}

func writeReportJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")

	return encoder.Encode(value)
}

func warnReportProblems(output io.Writer, problems []report.HistoryProblem) {
	for _, problem := range problems {
		fmt.Fprintf(output, "warning: report %q: %s\n", problem.ID, problem.Error)
	}
}

func writeReportEntries(output io.Writer, format string, entries []report.HistoryEntry) error {
	if format == "json" {
		return writeReportJSON(output, entries)
	}

	writer := tabwriter.NewWriter(output, 0, 0, reportColumnPadding, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tSTARTED_AT\tWORKLOAD\tDRIVER(S)\tSTATUS\tTPS"); err != nil {
		return err
	}

	for i := range entries {
		entry := &entries[i]

		drivers := []string{}
		for _, driver := range entry.Drivers {
			drivers = append(drivers, driver.Name+"="+driver.Type)
		}

		if len(drivers) == 0 {
			drivers = append(drivers, entry.Driver)
		}

		tps := "-"
		if entry.TPS != nil {
			tps = fmt.Sprintf("%.2f", *entry.TPS)
		}

		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			entry.ID, entry.StartedAt.UTC().Format(time.RFC3339Nano), entry.Workload,
			strings.Join(drivers, ","), entry.Status, tps,
		); err != nil {
			return err
		}
	}

	return writer.Flush()
}

func removeReports(cmd *cobra.Command, history *report.History, ids []string, yes bool, format string) error {
	if err := history.CheckRemoval(ids); err != nil {
		return err
	}

	selected := ids

	var removalErr error

	if yes {
		selected = []string{}

		for _, id := range ids {
			if err := history.Remove(id); err != nil {
				removalErr = errors.Join(removalErr, fmt.Errorf("remove report %q: %w", id, err))
			} else {
				selected = append(selected, id)
			}
		}
	}

	if format == "json" {
		return errors.Join(removalErr, writeReportJSON(cmd.OutOrStdout(), struct {
			IDs     []string `json:"ids"`
			Deleted bool     `json:"deleted"`
		}{selected, yes}))
	}

	action := "Would remove"
	if yes {
		action = "Removed"
	}

	for _, id := range selected {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", action, id); err != nil {
			return errors.Join(removalErr, err)
		}
	}

	if !yes {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%d reports selected; pass --yes to delete.\n", len(selected))

		return err
	}

	return removalErr
}
