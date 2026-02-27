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

type Subprobe struct {
	// Options is k6 export const options = { ... }
	Options *lib.Options `json:"options"`

	// Not nil if the test uses the parse_sql_with_groups function and accessing its groups/sections.
	// Like this sections["create_schema"]...
	SQLSections []string `json:"sql_sections"`

	// Envs is environment variables which test checks while execution.
	// It's a list of envs names (not K=V, just K)
	Envs []string `json:"envs"`

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
func (p *Probeprint) Explain() string { //nolint: gocognit // just fine
	sb := &strings.Builder{}

	sb.WriteString("Global Config:\n")

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

	sb.WriteString("K6 Options:\n")

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

	sb.WriteString("SQL Sections:\n")

	if len(p.SQLSections) > 0 {
		for _, section := range p.SQLSections {
			fmt.Fprintf(sb, "--+ %s\n", section)
		}
	} else {
		sb.WriteString("  (no sections)\n")
	}

	sb.WriteString("\n")

	sb.WriteString("Steps:\n")

	if len(p.Steps) > 0 {
		for _, step := range p.Steps {
			fmt.Fprintf(sb, "  %q\n", step)
		}
	} else {
		sb.WriteString("  (no sections)\n")
	}

	sb.WriteString("\n")

	sb.WriteString("Environment Variables:\n")

	if len(p.Envs) > 0 {
		for _, envName := range p.Envs {
			currentVal := os.Getenv(envName)
			if currentVal == "" {
				fmt.Fprintf(sb, "  %s=\"\"\n", envName)
			} else {
				fmt.Fprintf(sb, "  %s=%s\n", envName, currentVal)
			}
		}
	} else {
		sb.WriteString("  (no environment variables)\n")
	}

	return sb.String()
}
