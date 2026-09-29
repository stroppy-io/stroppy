package workloadcatalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

const stroppyModulePath = "github.com/stroppy-io/stroppy/v6"

var ErrReplaceConflict = errors.New("conflicting local module replacements")

// RunnerRequest describes one generated Stroppy executable.
type RunnerRequest struct {
	Packages        []Package
	IncludeBuiltIns bool
	Output          string
	TargetOS        string
	TargetArch      string
	Offline         bool
	Diagnostics     io.Writer
	StroppyRoot     string
}

// BuildRunner links selected workload packages into one Stroppy executable.
func BuildRunner(ctx context.Context, compiler *toolchain.Compiler, request RunnerRequest) error {
	if len(request.Packages) == 0 && !request.IncludeBuiltIns {
		return errors.New("runner contains no workloads")
	}

	replacements := map[string]string{}
	if request.StroppyRoot != "" {
		replacements[stroppyModulePath] = request.StroppyRoot
	}

	imports := make([]string, 0, len(request.Packages)+1)
	if request.IncludeBuiltIns {
		imports = append(imports, stroppyModulePath+"/workloads/all")
	}

	for _, pkg := range request.Packages {
		imports = append(imports, pkg.ImportPath)

		values, err := ModuleReplacements(ctx, compiler, pkg, request.Offline)
		if err != nil {
			return err
		}
		for module, path := range values {
			if existing, ok := replacements[module]; ok && existing != path {
				return fmt.Errorf("%w for %s: %s and %s", ErrReplaceConflict, module, existing, path)
			}
			replacements[module] = path
		}
	}

	temporary, err := os.MkdirTemp("", "stroppy-runner-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)

	if err := writeRunnerModule(temporary, imports, replacements); err != nil {
		return err
	}

	outputDirectory := filepath.Dir(request.Output)
	if err := os.MkdirAll(outputDirectory, 0o700); err != nil {
		return err
	}

	temporaryOutput, err := os.CreateTemp(outputDirectory, ".stroppy-export-*")
	if err != nil {
		return err
	}
	temporaryPath := temporaryOutput.Name()
	if err := temporaryOutput.Close(); err != nil {
		return err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return err
	}
	defer os.Remove(temporaryPath)

	command := exec.CommandContext(
		ctx, compiler.Path, "build", "-trimpath", "-mod=mod", "-o", temporaryPath, ".",
	)
	command.Dir = temporary
	command.Env = compiler.Env(request.TargetOS, request.TargetArch, request.Offline)
	command.Stdout = request.Diagnostics
	command.Stderr = request.Diagnostics
	if err := command.Run(); err != nil {
		return fmt.Errorf("build portable Stroppy: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o700); err != nil {
		return err
	}

	return os.Rename(temporaryPath, request.Output)
}

func writeRunnerModule(directory string, imports []string, replacements map[string]string) error {
	sort.Strings(imports)

	var mainSource strings.Builder
	mainSource.WriteString("package main\n\nimport (\n")
	mainSource.WriteString("\tstroppy \"")
	mainSource.WriteString(stroppyModulePath)
	mainSource.WriteString("\"\n")
	for _, importPath := range imports {
		fmt.Fprintf(&mainSource, "\t_ %q\n", importPath)
	}
	mainSource.WriteString(")\n\nfunc main() { stroppy.RegisteredMain() }\n")

	var module strings.Builder
	module.WriteString("module stroppy.local/export\n\ngo 1.26\n\n")
	fmt.Fprintf(&module, "require %s %s\n", stroppyModulePath, stroppyModuleVersion())

	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&module, "replace %s => %s\n", key, replacements[key])
	}

	if err := os.WriteFile(filepath.Join(directory, "main.go"), []byte(mainSource.String()), 0o600); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(directory, "go.mod"), []byte(module.String()), 0o600)
}
