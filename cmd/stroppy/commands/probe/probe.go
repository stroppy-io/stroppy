// Package probe lists embedded workload presets and driver capabilities.
package probe

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/pkg/bench"
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
and the insert methods each driver supports. Registered workload parameter schemas
are read without setting up a workload or connecting to a database.

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

				return printCatalog(cmd.OutOrStdout(), formatFlagValue)
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

// printCatalog renders the embedded preset catalog, workload schemas, and driver
// insert-method matrix in the requested format.
func printCatalog(output io.Writer, format string) error {
	catalog, err := workloads.Catalog()
	if err != nil {
		return fmt.Errorf("failed to build workloads catalog: %w", err)
	}

	descriptions, err := bench.DescribeAll()
	if err != nil {
		return fmt.Errorf("failed to describe workloads: %w", err)
	}

	drivers := driverCatalog()
	workloadSchemas := describeWorkloads(descriptions)

	switch format {
	case jsonFormat:
		bytes, err := json.Marshal(catalogOutput{
			Presets:   catalog,
			Drivers:   drivers,
			Workloads: workloadSchemas,
		})
		if err != nil {
			return fmt.Errorf("can't marshal catalog: %w", err)
		}

		fmt.Fprintf(output, "%s\n", string(bytes))
	case humanFormat:
		fmt.Fprint(output, formatCatalog(catalog, drivers, workloadSchemas))
	}

	return nil
}

type catalogOutput struct {
	Presets   []workloads.PresetInfo `json:"presets"`
	Drivers   []driverEntry          `json:"drivers"`
	Workloads []workloadEntry        `json:"workloads"`
}

type workloadEntry struct {
	Name   string       `json:"name"`
	Params []paramEntry `json:"params"`
}

type paramEntry struct {
	Name          string           `json:"name"`
	Flag          string           `json:"flag"`
	Scope         bench.ParamScope `json:"scope"`
	Type          bench.ParamType  `json:"type"`
	Description   string           `json:"description"`
	Default       any              `json:"default"`
	Env           string           `json:"env"`
	LegacyAliases []string         `json:"legacy_aliases"`
	Config        string           `json:"config"`
}

func describeWorkloads(descriptions []bench.Description) []workloadEntry {
	entries := make([]workloadEntry, 0, len(descriptions))

	for _, description := range descriptions {
		params := make([]paramEntry, 0, len(description.Params))
		for idx := range description.Params {
			param := &description.Params[idx]

			defaultValue := param.Default
			if param.Type == bench.ParamTypeDuration {
				defaultValue = fmt.Sprint(param.Default)
			}

			params = append(params, paramEntry{
				Name:          param.Name,
				Flag:          param.Flag,
				Scope:         param.Scope,
				Type:          param.Type,
				Description:   param.Description,
				Default:       defaultValue,
				Env:           param.Env,
				LegacyAliases: append([]string{}, param.LegacyEnvAliases...),
				Config:        param.Config,
			})
		}

		entries = append(entries, workloadEntry{Name: description.Name, Params: params})
	}

	return entries
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

// formatCatalog builds the human-readable preset listing, workload schemas,
// and driver insert-method matrix.
func formatCatalog(
	catalog []workloads.PresetInfo,
	drivers []driverEntry,
	workloadSchemas []workloadEntry,
) string {
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

	builder.WriteString("WORKLOADS (typed parameters)\n\n")

	for _, workload := range workloadSchemas {
		builder.WriteString("  " + workload.Name + "\n")

		for _, group := range []struct {
			label string
			scope bench.ParamScope
		}{
			{"run", bench.ParamScopeRun},
			{"workload", bench.ParamScopeWorkload},
		} {
			flags := paramFlags(workload.Params, group.scope)
			if len(flags) > 0 {
				fmt.Fprintf(&builder, "    %-9s %s\n", group.label+":", strings.Join(flags, ", "))
			}
		}

		builder.WriteString("\n")
	}

	builder.WriteString("  Use 'stroppy run <workload> --help' for types, defaults, and sources.\n\n")
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

func paramFlags(params []paramEntry, scope bench.ParamScope) []string {
	flags := make([]string, 0, len(params))
	for idx := range params {
		param := &params[idx]
		if param.Scope == scope {
			flags = append(flags, param.Flag)
		}
	}

	return flags
}
