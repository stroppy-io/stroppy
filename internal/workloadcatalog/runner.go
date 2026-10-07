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

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

const (
	stroppyModulePath = "github.com/stroppy-io/stroppy/v6"
	runnerDirPerm     = 0o700
	runnerFilePerm    = 0o600
)

var (
	ErrReplaceConflict = errors.New("conflicting local module replacements")
	errEmptyRunner     = errors.New("runner contains no workloads")
)

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
	CacheRoot       string
	BuildVersion    string
	BuildDigest     string
}

// BuildRunner links selected workload packages into one Stroppy executable.
func BuildRunner(ctx context.Context, compiler *toolchain.Compiler, request *RunnerRequest) error {
	if request.CacheRoot != "" {
		_, _, err := BuildCached(ctx, compiler, request)

		return err
	}

	return buildRunnerUncached(ctx, compiler, request)
}

func buildRunnerUncached(ctx context.Context, compiler *toolchain.Compiler, request *RunnerRequest) error {
	if len(request.Packages) == 0 && !request.IncludeBuiltIns {
		return errEmptyRunner
	}

	mainSource, moduleSource, err := runnerSources(
		request.Packages, request.IncludeBuiltIns, request.StroppyRoot,
		request.BuildVersion, request.BuildDigest,
	)
	if err != nil {
		return err
	}

	temporary, err := os.MkdirTemp("", "stroppy-runner-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)

	if err := writeRunnerSources(temporary, mainSource, moduleSource); err != nil {
		return err
	}

	outputDirectory := filepath.Dir(request.Output)
	if err := os.MkdirAll(outputDirectory, runnerDirPerm); err != nil {
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

	arguments := []string{"build", "-trimpath", "-mod=mod", "-o", temporaryPath, "."}

	command := exec.CommandContext( //nolint:gosec // compiler path comes from verified resolver
		ctx, compiler.Path, arguments...,
	)
	command.Dir = temporary
	command.Env = compiler.Env(request.TargetOS, request.TargetArch, request.Offline)
	command.Stdout = request.Diagnostics

	command.Stderr = request.Diagnostics
	if err := command.Run(); err != nil {
		return fmt.Errorf("build portable Stroppy: %w", err)
	}

	if err := os.Chmod(temporaryPath, runnerDirPerm); err != nil {
		return err
	}

	return os.Rename(temporaryPath, request.Output)
}

func runnerSources(
	packages []Package,
	includeBuiltIns bool,
	stroppyRoot, buildVersion, buildDigest string,
) (mainSource, moduleSource []byte, err error) {
	requirements := map[string]string{stroppyModulePath: stroppyModuleVersion()}

	replacements := moduleReplacements{}
	if stroppyRoot != "" {
		replacements[module.Version{Path: stroppyModulePath}] = module.Version{Path: stroppyRoot}
	}

	imports := make([]string, 0, len(packages)+1)
	if includeBuiltIns {
		imports = append(imports, stroppyModulePath+"/workloads/all")
	}

	for _, pkg := range packages {
		imports = append(imports, pkg.ImportPath)

		packageRequirements, packageReplacements, err := ModuleConfig(&pkg)
		if err != nil {
			return nil, nil, err
		}

		mergeRequirements(requirements, packageRequirements)

		if err := mergeReplacements(replacements, packageReplacements); err != nil {
			return nil, nil, err
		}
	}

	if buildVersion == "" {
		buildVersion = stroppyVersion()
	}

	return formatRunnerSources(imports, requirements, replacements, buildVersion, buildDigest)
}

func mergeRequirements(destination, source map[string]string) {
	for path, version := range source {
		existing, ok := destination[path]
		if !ok || semver.Compare(version, existing) > 0 {
			destination[path] = version
		}
	}
}

func mergeReplacements(destination, source moduleReplacements) error {
	for oldModule, newModule := range source {
		if existing, ok := destination[oldModule]; ok && existing != newModule {
			return fmt.Errorf(
				"%w for %s: %s and %s", ErrReplaceConflict,
				oldModule.String(), existing.String(), newModule.String(),
			)
		}

		destination[oldModule] = newModule
	}

	return nil
}

func formatRunnerSources(
	imports []string,
	requirements map[string]string,
	replacements moduleReplacements,
	buildVersion, buildDigest string,
) (main, moduleSource []byte, err error) {
	sort.Strings(imports)

	var mainSource strings.Builder
	mainSource.WriteString("package main\n\nimport (\n")
	mainSource.WriteString("\tstroppy \"")
	mainSource.WriteString(stroppyModulePath)
	mainSource.WriteString("\"\n")

	for _, importPath := range imports {
		fmt.Fprintf(&mainSource, "\t_ %q\n", importPath)
	}

	mainSource.WriteString(")\n\nfunc main() { stroppy.RegisteredMain(")
	fmt.Fprintf(&mainSource, "%q, %q", buildVersion, buildDigest)
	mainSource.WriteString(") }\n")

	moduleFile := new(modfile.File)
	if err := moduleFile.AddModuleStmt("stroppy.local/export"); err != nil {
		return nil, nil, err
	}

	if err := moduleFile.AddGoStmt("1.27"); err != nil {
		return nil, nil, err
	}

	requirementKeys := make([]string, 0, len(requirements))
	for key := range requirements {
		requirementKeys = append(requirementKeys, key)
	}

	sort.Strings(requirementKeys)

	for _, key := range requirementKeys {
		if err := moduleFile.AddRequire(key, requirements[key]); err != nil {
			return nil, nil, err
		}
	}

	replacementKeys := make([]module.Version, 0, len(replacements))
	for key := range replacements {
		replacementKeys = append(replacementKeys, key)
	}

	sort.Slice(replacementKeys, func(left, right int) bool {
		return replacementKeys[left].String() < replacementKeys[right].String()
	})

	for _, oldModule := range replacementKeys {
		newModule := replacements[oldModule]
		if err := moduleFile.AddReplace(
			oldModule.Path, oldModule.Version, newModule.Path, newModule.Version,
		); err != nil {
			return nil, nil, err
		}
	}

	moduleSource, err = moduleFile.Format()
	if err != nil {
		return nil, nil, err
	}

	return []byte(mainSource.String()), moduleSource, nil
}

func writeRunnerSources(directory string, mainSource, moduleSource []byte) error {
	if err := os.WriteFile(filepath.Join(directory, "main.go"), mainSource, runnerFilePerm); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(directory, "go.mod"), moduleSource, runnerFilePerm)
}

func writeRunnerModule(
	directory string,
	imports []string,
	requirements map[string]string,
	replacements moduleReplacements,
) error {
	mainSource, moduleSource, err := formatRunnerSources(
		imports, requirements, replacements, stroppyVersion(), "",
	)
	if err != nil {
		return err
	}

	return writeRunnerSources(directory, mainSource, moduleSource)
}
