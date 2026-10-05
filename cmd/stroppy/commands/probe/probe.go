// Package probe lists embedded workload presets and driver capabilities.
package probe

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	runcommand "github.com/stroppy-io/stroppy/v6/cmd/stroppy/commands/run"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/workloads"
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
	Cmd                 = NewCommand(bench.RegisteredCatalog())
)

// NewCommand builds a probe command over an explicit workload catalog.
//
//nolint:gocognit // catalog and selected/resolved views share one command boundary.
func NewCommand(workloadCatalog *bench.Catalog) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "probe [workload] [--resolved] [run inputs]",
		Short: "Describe workloads, embedded presets, and supported drivers",
		Long: `Probe lists the embedded workload presets (their SQL dialects and docs)
and the insert methods each driver supports. Registered workload parameter schemas
are read without setting up a workload or connecting to a database.

  -o json   machine-readable output
`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if slices.Contains(args, "--help") || slices.Contains(args, "-h") {
				return cmd.Help()
			}

			name, formatFlagValue, resolved, inputs, err := parseProbeArgs(args)
			if err != nil {
				return err
			}

			if !contains(formats, formatFlagValue) {
				return fmt.Errorf(
					"%q, available (%s): %w",
					formatFlagValue,
					formatsWithCommas,
					ErrUnsoportedFormat,
				)
			}

			if name == "" {
				if resolved || len(inputs) != 0 {
					return errProbeSelection
				}

				return printCatalog(cmd.OutOrStdout(), workloadCatalog, formatFlagValue)
			}

			if !resolved && len(inputs) != 0 {
				return errProbeResolution
			}

			var description bench.Description
			if resolved {
				description, err = runcommand.ResolveDescription(workloadCatalog, name, inputs)
			} else {
				description, err = workloadCatalog.Describe(name)
			}

			if err != nil {
				return err
			}

			return printDescriptions(cmd.OutOrStdout(), formatFlagValue, []bench.Description{description}, nil)
		},
	}

	cmd.Flags().StringP(
		formatFlag,
		string(formatFlag[0]),
		humanFormat,
		fmt.Sprintf("(%s)", formatsWithCommas),
	)

	cmd.Flags().Bool("resolved", false, "Resolve inputs and show effective values without executing actions")

	return cmd
}

var (
	errProbeSelection  = errors.New("resolved probe requires a workload name")
	errProbeResolution = errors.New("probe run inputs require --resolved")
	errProbeOutput     = errors.New("probe output requires a value")
)

func parseProbeArgs(args []string) (name, format string, resolved bool, inputs []string, err error) {
	format = humanFormat

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--resolved":
			resolved = true
		case arg == "-o" || arg == "--output":
			index++
			if index == len(args) {
				return "", "", false, nil, errProbeOutput
			}

			format = args[index]
		case strings.HasPrefix(arg, "--output="):
			format = strings.TrimPrefix(arg, "--output=")
		case name == "" && !strings.HasPrefix(arg, "-"):
			name = arg
		default:
			inputs = append(inputs, arg)
		}
	}

	return name, format, resolved, inputs, err
}

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
func printCatalog(output io.Writer, workloadCatalog *bench.Catalog, format string) error {
	catalog, err := workloads.Catalog()
	if err != nil {
		return fmt.Errorf("failed to build workloads catalog: %w", err)
	}

	descriptions, err := workloadCatalog.DescribeAll()
	if err != nil {
		return fmt.Errorf("failed to describe workloads: %w", err)
	}

	return printDescriptions(output, format, descriptions, catalog)
}

func printDescriptions(
	output io.Writer,
	format string,
	descriptions []bench.Description,
	catalog []workloads.PresetInfo,
) error {
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
	Name       string                    `json:"name"`
	Params     []paramEntry              `json:"params"`
	Steps      []bench.StepDescription   `json:"steps"`
	DriverRefs []bench.DriverDescription `json:"driver_refs"`
	Metrics    []bench.MetricSchema      `json:"metrics"`
	Values     map[string]valueEntry     `json:"values,omitempty"`
}

type valueEntry struct {
	Value    any               `json:"value"`
	Source   bench.ParamSource `json:"source"`
	Spelling string            `json:"spelling"`
}

type paramEntry struct {
	Name               string             `json:"name"`
	Flag               string             `json:"flag"`
	Scope              bench.ParamScope   `json:"scope"`
	Type               bench.ParamType    `json:"type"`
	Description        string             `json:"description"`
	Default            any                `json:"default"`
	DefaultDescription string             `json:"default_description,omitempty"`
	Env                string             `json:"env"`
	Aliases            []string           `json:"aliases"`
	Config             string             `json:"config"`
	Constraints        []bench.Constraint `json:"constraints"`
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
				Name:               param.Name,
				Flag:               param.Flag,
				Scope:              param.Scope,
				Type:               param.Type,
				Description:        param.Description,
				Default:            defaultValue,
				DefaultDescription: param.DefaultDescription,
				Env:                param.Env,
				Aliases:            append([]string{}, param.Aliases...),
				Config:             param.Config,
				Constraints:        append([]bench.Constraint{}, param.Constraints...),
			})
		}

		values := map[string]valueEntry{}

		for name, value := range description.Values {
			resolved := value.Value
			if duration, ok := resolved.(time.Duration); ok {
				resolved = duration.String()
			}

			values[name] = valueEntry{resolved, value.Info.Source, value.Info.Spelling}
		}

		entries = append(entries, workloadEntry{
			Name: description.Name, Params: params, Steps: description.Steps,
			DriverRefs: description.Drivers, Metrics: description.Metrics, Values: values,
		})
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

		for _, step := range workload.Steps {
			fmt.Fprintf(&builder, "    step: %s (%s, workers=%d, measured=%t)\n",
				step.Name, step.Executor, step.Workers, step.Measured)
		}

		for _, ref := range workload.DriverRefs {
			fmt.Fprintf(&builder, "    database: %s (%s)\n", ref.Name, ref.Kind)
		}

		for _, name := range slices.Sorted(maps.Keys(workload.Values)) {
			value := workload.Values[name]
			fmt.Fprintf(&builder, "    %s=%v (%s: %s)\n", name, value.Value, value.Source, value.Spelling)
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
