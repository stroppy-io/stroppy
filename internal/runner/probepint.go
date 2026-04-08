package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"go.k6.io/k6/lib"
	"google.golang.org/protobuf/encoding/protojson"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

type SQLQuery struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

type SQLSection struct {
	Name    string     `json:"name"`
	Queries []SQLQuery `json:"queries"`
}

// EnvDeclaration captures metadata from ENV() calls in user scripts.
type EnvDeclaration struct {
	Names       []string `json:"names"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
}

// DriverSetupDecl captures a declareDriverSetup(index, defaults) call from the script.
type DriverSetupDecl struct {
	Index    int            `json:"index"`
	Defaults map[string]any `json:"defaults"`
}

type Subprobe struct {
	// Options is k6 export const options = { ... }
	Options *lib.Options `json:"options"`

	// Not nil if the test uses the parse_sql_with_groups function and accessing its groups/sections.
	// Like this sections["create_schema"]...
	SQLSections []SQLSection `json:"sql_sections"`

	// Envs is environment variables accessed via __ENV directly (legacy).
	Envs []string `json:"envs"`

	// EnvDeclarations is environment variables declared via ENV() with metadata.
	EnvDeclarations []EnvDeclaration `json:"env_declarations"`

	// Steps is which ones registered with 'Step("", ()=>{})' function.
	Steps []string `json:"steps"`

	// Drivers is configuration passed to each DriverX.create().setup({...}) call.
	// Serialized via protojson in Probeprint.MarshalJSON.
	Drivers []*stroppy.DriverConfig `json:"-"`

	// DriverSetups captures declareDriverSetup() calls — human-readable driver defaults from the script.
	DriverSetups []DriverSetupDecl `json:"driver_setups"`
}

// Probeprint contains configuration and other metainformation extracted from a TypeScript script.
type Probeprint struct {
	// GlobalConfig is config passed to driver(s) while test.
	GlobalConfig *stroppy.GlobalConfig `json:"global_config"`

	// FileConfig is the user config file loaded via -f/--file; not serialized.
	FileConfig *stroppy.RunConfig `json:"-"`

	Subprobe
}

var _ json.Marshaler = new(Probeprint)

func (p *Probeprint) MarshalJSON() ([]byte, error) {
	buff := &bytes.Buffer{}
	_, _ = buff.WriteString(`{"global_config":`)

	configJSON, err := protojson.MarshalOptions{}.Marshal(p.GlobalConfig)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling Probeprint.GlobalConfig: %w", err)
	}

	_, _ = buff.Write(configJSON)
	_, _ = buff.WriteString(",")

	subprobeJSON, err := json.Marshal(p.Subprobe)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling Probeprint.Subprobe: %w", err)
	}

	// subprobeJSON is "{...}", strip trailing "}" to append drivers
	_, _ = buff.Write(subprobeJSON[1 : len(subprobeJSON)-1])

	// append drivers via protojson
	opts := protojson.MarshalOptions{}

	_, _ = buff.WriteString(`,"drivers":[`)

	for i, dc := range p.Drivers {
		if i > 0 {
			_, _ = buff.WriteString(",")
		}

		driverJSON, err := opts.Marshal(dc)
		if err != nil {
			return nil, fmt.Errorf("failed marshaling DriverConfig: %w", err)
		}

		_, _ = buff.Write(driverJSON)
	}

	_, _ = buff.WriteString("]}")

	return buff.Bytes(), nil
}

// ExplainSection is a bitmask for selecting which sections to include in Explain output.
type ExplainSection uint8

const (
	ExplainConfig ExplainSection = 1 << iota
	ExplainOptions
	ExplainSQL
	ExplainSteps
	ExplainEnvs
	ExplainDrivers

	ExplainAll ExplainSection = ExplainConfig | ExplainOptions | ExplainSQL | ExplainSteps | ExplainEnvs | ExplainDrivers
)

// Explain returns a human-readable message for users.
// Use sections bitmask to select which parts to include.
func (p *Probeprint) Explain(sections ExplainSection) string {
	sb := &strings.Builder{}

	sb.WriteString("Use 'stroppy help probe' to get details about sections\n\n")

	if sections&ExplainConfig != 0 {
		p.explainConfig(sb)
	}

	if sections&ExplainOptions != 0 {
		p.explainOptions(sb)
	}

	if sections&ExplainSQL != 0 {
		p.explainSQL(sb)
	}

	if sections&ExplainSteps != 0 {
		p.explainSteps(sb)
	}

	if sections&ExplainEnvs != 0 {
		p.explainEnvs(sb)
	}

	if sections&ExplainDrivers != 0 {
		p.explainDrivers(sb)
	}

	return sb.String()
}

func (p *Probeprint) explainConfig(sb *strings.Builder) {
	sb.WriteString("# Stroppy Config:\n")

	gc := p.GlobalConfig

	source := ""

	if p.FileConfig.GetGlobal() != nil && !isEmptyGlobalConfig(p.FileConfig.GetGlobal()) {
		gc = p.FileConfig.GetGlobal()
		source = " (from config file)"
	}

	if gc != nil {
		if source != "" {
			sb.WriteString(source)
			sb.WriteString("\n")
		}

		configJSON, err := protojson.MarshalOptions{
			Multiline:    true,
			AllowPartial: true,
			Indent:       "  ",
		}.Marshal(gc)
		if err != nil {
			panic(err)
		}

		sb.Write(configJSON)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("  (no config)\n\n")
	}
}

// isEmptyGlobalConfig reports whether gc has no meaningful fields set.
func isEmptyGlobalConfig(gc *stroppy.GlobalConfig) bool {
	return gc == nil || (gc.GetLogger() == nil && gc.GetExporter() == nil &&
		gc.GetRunId() == "" && gc.GetSeed() == 0 && len(gc.GetMetadata()) == 0)
}

func (p *Probeprint) explainOptions(sb *strings.Builder) {
	sb.WriteString("# K6 Options:\n")

	if p.Options != nil {
		optionsJSON, err := json.MarshalIndent(p.Options, "", "  ")
		if err != nil {
			panic(err)
		}

		optionsJSON = []byte(strings.Join(
			slices.DeleteFunc(
				strings.Split(string(optionsJSON), "\n"),
				func(s string) bool { return strings.Contains(s, ": null") },
			), "\n"))

		sb.Write(optionsJSON)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("  (no options)\n\n")
	}
}

func (p *Probeprint) explainSQL(sb *strings.Builder) {
	sb.WriteString("# SQL File Structure:\n")

	if len(p.SQLSections) > 0 {
		for _, section := range p.SQLSections {
			if section.Name != "" {
				fmt.Fprintf(sb, "  --+ %s\n", section.Name)
			} else {
				sb.WriteString("  (queries without named section)\n")
			}

			for _, query := range section.Queries {
				fmt.Fprintf(sb, "  --= %s\n", query.Name)
			}
		}
	} else {
		sb.WriteString("  (no sql requirements)\n")
	}

	sb.WriteString("\n")
}

func (p *Probeprint) explainSteps(sb *strings.Builder) {
	sb.WriteString("# Steps\n")

	if len(p.Steps) > 0 {
		for _, step := range p.Steps {
			fmt.Fprintf(sb, "  %q\n", step)
		}
	} else {
		sb.WriteString("  (no sections)\n")
	}

	sb.WriteString("\n")
}

func (p *Probeprint) explainEnvs(sb *strings.Builder) {
	sb.WriteString("# Environment Variables:\n")

	for _, decl := range p.EnvDeclarations {
		explainEnvDecl(sb, decl)
	}

	hasPlain := p.explainPlainEnvs(sb)

	if len(p.EnvDeclarations) == 0 && !hasPlain {
		sb.WriteString("  (no environment variables)\n")
	}

	if p.FileConfig != nil && len(p.FileConfig.GetEnv()) > 0 {
		sb.WriteString("\n  # From config file (-f):\n")

		for k, v := range p.FileConfig.GetEnv() {
			fmt.Fprintf(sb, "  %s=%s\n", k, v)
		}
	}

	sb.WriteString("\n")
}

func explainEnvDecl(sb *strings.Builder, decl EnvDeclaration) {
	names := strings.Join(decl.Names, " | ")
	currentVal := lookupEnv(decl.Names)

	switch {
	case currentVal != "":
		fmt.Fprintf(sb, "  %s=%s", names, currentVal)
	case decl.Default != "":
		fmt.Fprintf(sb, "  %s=\"\" (default: %s)", names, decl.Default)
	default:
		fmt.Fprintf(sb, "  %s=\"\"", names)
	}

	if decl.Description != "" {
		fmt.Fprintf(sb, "  # %s", decl.Description)
	}

	sb.WriteString("\n")
}

func lookupEnv(names []string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}

	return ""
}

func (p *Probeprint) explainDrivers(sb *strings.Builder) {
	sb.WriteString("# Drivers:\n")

	if len(p.DriverSetups) == 0 {
		sb.WriteString("  (no drivers)\n\n")

		return
	}

	for _, ds := range p.DriverSetups {
		if len(p.DriverSetups) > 1 {
			fmt.Fprintf(sb, "  ## Driver %d:\n", ds.Index)
		}

		data, err := json.MarshalIndent(ds.Defaults, "  ", "  ")
		if err != nil {
			panic(err)
		}

		sb.WriteString("  ")
		sb.Write(data)
		sb.WriteString("\n\n")
	}
}

// explainPlainEnvs writes env vars accessed via __ENV directly (not via ENV()).
// Returns true if any plain env vars were written.
func (p *Probeprint) explainPlainEnvs(sb *strings.Builder) bool {
	declared := map[string]bool{}

	for _, decl := range p.EnvDeclarations {
		for _, name := range decl.Names {
			declared[name] = true
		}
	}

	hasPlain := false

	for _, envName := range p.Envs {
		if declared[envName] {
			continue
		}

		currentVal := os.Getenv(envName)
		if currentVal == "" {
			fmt.Fprintf(sb, "  %s=\"\"\n", envName)
		} else {
			fmt.Fprintf(sb, "  %s=%s\n", envName, currentVal)
		}

		hasPlain = true
	}

	return hasPlain
}
