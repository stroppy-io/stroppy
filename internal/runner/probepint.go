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
}

// Probeprint contains configuration and other metainformation extracted from a TypeScript script.
type Probeprint struct {
	// GlobalConfig is config passed to driver(s) while test.
	GlobalConfig *stroppy.GlobalConfig `json:"global_config"`

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

	_, _ = buff.Write(subprobeJSON[1:])

	return buff.Bytes(), nil
}

// Explain - human-readable message for users.
// TODO: Explain(parts (config|options|sql|envs)) feature flags (bit-flags) format.
func (p *Probeprint) Explain() string { //nolint: gocognit // just fine
	sb := &strings.Builder{}

	sb.WriteString("Use 'probe --help' to get details about sections\n\n")

	sb.WriteString("# Stroppy Config:\n")

	if p.GlobalConfig != nil {
		configJSON, err := protojson.MarshalOptions{
			Multiline:    true,
			AllowPartial: true,
			Indent:       "  ",
		}.Marshal(
			p.GlobalConfig,
		)
		if err != nil {
			panic(err)
		}

		sb.Write(configJSON)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("  (no config)\n\n")
	}

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

	sb.WriteString("# Steps\n")

	if len(p.Steps) > 0 {
		for _, step := range p.Steps {
			fmt.Fprintf(sb, "  %q\n", step)
		}
	} else {
		sb.WriteString("  (no sections)\n")
	}

	sb.WriteString("\n")

	sb.WriteString("# Environment Variables:\n")

	if len(p.EnvDeclarations) > 0 {
		for _, decl := range p.EnvDeclarations {
			names := strings.Join(decl.Names, " | ")
			currentVal := ""

			for _, name := range decl.Names {
				if v := os.Getenv(name); v != "" {
					currentVal = v

					break
				}
			}

			if currentVal != "" {
				fmt.Fprintf(sb, "  %s=%s", names, currentVal)
			} else if decl.Default != "" {
				fmt.Fprintf(sb, "  %s=\"\" (default: %s)", names, decl.Default)
			} else {
				fmt.Fprintf(sb, "  %s=\"\"", names)
			}

			if decl.Description != "" {
				fmt.Fprintf(sb, "  # %s", decl.Description)
			}

			sb.WriteString("\n")
		}
	}

	// Plain __ENV access (not via ENV())
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

	if len(p.EnvDeclarations) == 0 && !hasPlain {
		sb.WriteString("  (no environment variables)\n")
	}

	return sb.String()
}
