// Package probe lists embedded workload presets and driver capabilities.
package probe

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/workloads"
)

const (
	formatFlag = "output"

	humanFormat = "human"
	jsonFormat  = "json"
)

var (
	formats             = []string{humanFormat, jsonFormat}
	formatsWithCommas   = strings.Join(formats, ", ")
	ErrUnsoportedFormat = errors.New("unsupported format")
	Cmd                 = func() *cobra.Command {
		cmd := &cobra.Command{
			Use:   "probe",
			Short: "List embedded workload presets and supported drivers",
			Long: `Probe lists the embedded workload presets (their SQL dialects and docs)
and the insert methods each driver supports.

  -o json   machine-readable output
`,
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				formatFlagValue := cmd.Flag(formatFlag).Value.String()

				if !contains(formats, formatFlagValue) {
					return fmt.Errorf(
						"%q, available (%s): %w",
						formatFlagValue,
						formatsWithCommas,
						ErrUnsoportedFormat,
					)
				}

				return printCatalog(formatFlagValue)
			},
		}

		cmd.Flags().
			StringP(formatFlag, string(formatFlag[0]), humanFormat,
				fmt.Sprintf("(%s)", formatsWithCommas))

		return cmd
	}()
)

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}

	return false
}

// printCatalog renders the embedded preset catalog and the driver
// insert-method matrix in the requested format.
func printCatalog(format string) error {
	catalog, err := workloads.Catalog()
	if err != nil {
		return fmt.Errorf("failed to build workloads catalog: %w", err)
	}

	drivers := driverCatalog()

	switch format {
	case jsonFormat:
		bytes, err := json.Marshal(map[string]any{"presets": catalog, "drivers": drivers})
		if err != nil {
			return fmt.Errorf("can't marshal catalog: %w", err)
		}

		fmt.Fprintf(os.Stdout, "%s\n", string(bytes))
	case humanFormat:
		fmt.Fprint(os.Stdout, formatCatalog(catalog, drivers))
	}

	return nil
}

// driverEntry is one row of the driver capability matrix in catalog output.
type driverEntry struct {
	Type          string   `json:"type"`
	InsertMethods []string `json:"insert_methods"`
}

// driverCatalog converts the static driver→insert-method matrix to
// lowercase names ("postgres", "plain_bulk") for catalog output.
func driverCatalog() []driverEntry {
	capabilities := driver.InsertCapabilities()

	entries := make([]driverEntry, 0, len(capabilities))

	for _, capability := range capabilities {
		methods := make([]string, 0, len(capability.InsertMethods))
		for _, method := range capability.InsertMethods {
			methods = append(methods, strings.ToLower(method.String()))
		}

		entries = append(entries, driverEntry{
			Type: strings.ToLower(
				strings.TrimPrefix(capability.Type.String(), "DRIVER_TYPE_"),
			),
			InsertMethods: methods,
		})
	}

	return entries
}

// formatCatalog builds the human-readable preset listing and driver
// insert-method matrix.
func formatCatalog(catalog []workloads.PresetInfo, drivers []driverEntry) string {
	var builder strings.Builder

	builder.WriteString("\nPRESETS (embedded workloads)\n\n")

	for _, preset := range catalog {
		builder.WriteString("  " + preset.Name + "\n")

		if len(preset.SQL) > 0 {
			builder.WriteString("    sql:   " + strings.Join(preset.SQL, ", ") + "\n")
		}

		if len(preset.Docs) > 0 {
			builder.WriteString("    docs:  " + strings.Join(preset.Docs, ", ") + "\n")
		}

		builder.WriteString("\n")
	}

	builder.WriteString("DRIVERS (supported insert methods)\n\n")

	typeWidth := 0
	for _, entry := range drivers {
		typeWidth = max(typeWidth, len(entry.Type))
	}

	for _, entry := range drivers {
		fmt.Fprintf(&builder, "  %-*s  %s\n",
			typeWidth, entry.Type, strings.Join(entry.InsertMethods, ", "))
	}

	builder.WriteString("\n")

	return builder.String()
}
