package workloadcatalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
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
	Package   Package
	temporary string
}

// Build compiles one importable workload package through a generated runner.
func Build(
	ctx context.Context,
	compiler *toolchain.Compiler,
	source string,
	diagnostics io.Writer,
	offline bool,
	stroppyRoot string,
) (BuildResult, error) {
	pkg, err := DiscoverPackage(ctx, compiler, source, offline)
	if err != nil {
		return BuildResult{}, err
	}

	temporary, err := os.MkdirTemp("", "stroppy-workload-build-*")
	if err != nil {
		return BuildResult{}, fmt.Errorf("create build directory: %w", err)
	}

	result := BuildResult{
		Source:    pkg.Directory,
		Artifact:  filepath.Join(temporary, "workload"),
		Package:   pkg,
		temporary: temporary,
	}

	if err := BuildRunner(ctx, compiler, RunnerRequest{
		Packages:    []Package{pkg},
		Output:      result.Artifact,
		Offline:     offline,
		Diagnostics: diagnostics,
		StroppyRoot: stroppyRoot,
	}); err != nil {
		result.Cleanup()

		return BuildResult{}, err
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
