package workloadcatalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	ErrGoUnavailable = errors.New("compatible Go toolchain is unavailable")
	ErrSourceProject = errors.New("workload source must be a Go project directory")
)

// BuildResult is one compiled and probed standalone workload.
type BuildResult struct {
	Name      string
	Source    string
	Artifact  string
	temporary string
}

// Build compiles one standalone workload project with system Go and probes its identity.
func Build(ctx context.Context, source string, diagnostics io.Writer) (BuildResult, error) {
	absolute, err := filepath.Abs(source)
	if err != nil {
		return BuildResult{}, fmt.Errorf("resolve workload source: %w", err)
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return BuildResult{}, fmt.Errorf("inspect workload source: %w", err)
	}

	if !info.IsDir() {
		return BuildResult{}, fmt.Errorf("%w: %s", ErrSourceProject, absolute)
	}

	if _, err := os.Stat(filepath.Join(absolute, "go.mod")); err != nil {
		return BuildResult{}, fmt.Errorf("%w: missing go.mod in %s", ErrSourceProject, absolute)
	}

	goBinary, err := exec.LookPath("go")
	if err != nil {
		return BuildResult{}, errors.Join(ErrGoUnavailable, err)
	}

	temporary, err := os.MkdirTemp("", "stroppy-workload-build-*")
	if err != nil {
		return BuildResult{}, fmt.Errorf("create build directory: %w", err)
	}

	result := BuildResult{
		Source:    absolute,
		Artifact:  filepath.Join(temporary, "workload"),
		temporary: temporary,
	}

	command := exec.CommandContext(ctx, goBinary, "build", "-trimpath", "-mod=mod", "-o", result.Artifact, ".")
	command.Dir = absolute
	command.Stdout = diagnostics
	command.Stderr = diagnostics

	command.Env = append(os.Environ(), "GOTOOLCHAIN=local")

	if err := command.Run(); err != nil {
		result.Cleanup()

		return BuildResult{}, fmt.Errorf("compile workload: %w", err)
	}

	probe, err := Probe(ctx, result.Artifact)
	if err != nil {
		result.Cleanup()

		return BuildResult{}, err
	}

	result.Name = probe.Name

	return result, nil
}

// Cleanup removes temporary build output.
func (result *BuildResult) Cleanup() {
	if result == nil || result.temporary == "" {
		return
	}

	_ = os.RemoveAll(result.temporary)
	result.temporary = ""
}
