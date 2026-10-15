package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/pkg/report/compare"
	"github.com/stroppy-io/stroppy/v6/pkg/report/view"
)

var errComparisonFormat = errors.New("unknown comparison output format")

func newReportsCompareCommand() *cobra.Command {
	var format, path, reference string

	command := &cobra.Command{
		Use: "compare ID [ID...]", Short: "Compare 2–8 saved reports against a reference run",
		Args: cobra.RangeArgs(compare.MinRuns, compare.MaxRuns),
		RunE: func(cmd *cobra.Command, ids []string) error {
			if format != "human" && format != "json" && format != "html" {
				return fmt.Errorf("%w: %q", errComparisonFormat, format)
			}

			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			result, err := compare.Load(history, ids, reference)
			if err != nil {
				return err
			}

			write := func(output io.Writer) error {
				switch format {
				case "json":
					encoder := json.NewEncoder(output)
					encoder.SetIndent("", "  ")

					return encoder.Encode(result)
				case "html":
					return view.RenderComparison(output, result, history.Directory, nil, false)
				default:
					return compare.WriteText(output, result)
				}
			}
			if path == "" {
				return write(cmd.OutOrStdout())
			}

			for _, run := range result.Runs {
				if err := checkReportExport(path, history, run.ID, run.Source); err != nil {
					return err
				}
			}

			return writeComparisonFile(path, write)
		},
	}
	command.Flags().StringVarP(&format, "output", "o", "human", "output format: human, json, html")
	command.Flags().StringVar(&path, "file", "", "atomic output file; omit for stdout")
	command.Flags().StringVar(&reference, "reference", "", "reference history ID; default first selected report")

	return command
}

func writeComparisonFile(path string, write func(io.Writer) error) (err error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".stroppy-comparison-*")
	if err != nil {
		return err
	}

	name := file.Name()

	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close(), os.Remove(name))
		}
	}()

	if err := write(file); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(name, path)
}
