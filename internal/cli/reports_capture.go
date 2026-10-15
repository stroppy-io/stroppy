package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/pkg/report/view"
)

const artifactArguments = 2

func checkReportExport(path string, history *report.History, id string, data []byte) error {
	same, err := report.SameOutput(path, filepath.Join(history.Directory, id+".json"))
	if err != nil {
		return err
	}

	if same {
		return errReportExportSource
	}

	capture, err := report.CaptureFromJSON(data)
	if err != nil {
		return err
	}

	same, err = report.OutputOverwritesCapture(path, history.Directory, capture)
	if err != nil {
		return err
	}

	if same {
		return errReportExportSource
	}

	return nil
}

func newReportsArtifactCommand() *cobra.Command {
	return &cobra.Command{
		Use: "artifact ID NAME", Short: "Retrieve a declared capture artifact", Args: cobra.ExactArgs(artifactArguments),
		RunE: func(cmd *cobra.Command, args []string) error {
			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			file, err := history.OpenArtifact(args[0], args[1])
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(cmd.OutOrStdout(), file)

			return err
		},
	}
}

func newReportsBundleCommand() *cobra.Command {
	var path string

	command := &cobra.Command{
		Use: "bundle ID", Short: "Export report and complete captures as a portable ZIP", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			if path == "" {
				return view.Bundle(cmd.OutOrStdout(), history, args[0])
			}

			data, err := history.Get(args[0])
			if err != nil {
				return err
			}

			if err := checkReportExport(path, history, args[0], data); err != nil {
				return err
			}

			file, err := os.CreateTemp(filepath.Dir(path), ".stroppy-bundle-*")
			if err != nil {
				return err
			}

			temporary := file.Name()

			defer func() {
				if err != nil {
					err = errors.Join(err, file.Close(), os.Remove(temporary))
				}
			}()

			if err := view.Bundle(file, history, args[0]); err != nil {
				return err
			}

			if err := file.Close(); err != nil {
				return err
			}

			return os.Rename(temporary, path)
		},
	}
	command.Flags().StringVar(&path, "file", "", "ZIP output path; omit for stdout")

	return command
}
