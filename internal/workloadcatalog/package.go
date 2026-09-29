package workloadcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

var (
	ErrMainPackage       = errors.New("workload package must be importable, not package main")
	ErrMissingModulePath = errors.New("workload package has no module path")
)

//nolint:tagliatelle // Go command JSON uses exported Go field names.
type goListPackage struct {
	ImportPath string        `json:"ImportPath"`
	Dir        string        `json:"Dir"`
	Name       string        `json:"Name"`
	Module     *goListModule `json:"Module"`
}

//nolint:tagliatelle // Go command JSON uses exported Go field names.
type goListModule struct {
	Path string `json:"Path"`
	Dir  string `json:"Dir"`
}

// Package describes one importable workload package and its module.
type Package struct {
	ImportPath string
	Directory  string
	ModulePath string
	ModuleRoot string
}

// DiscoverPackage resolves an importable workload package through Go tooling.
func DiscoverPackage(
	ctx context.Context,
	compiler *toolchain.Compiler,
	source string,
	offline bool,
) (Package, error) {
	absolute, err := filepath.Abs(source)
	if err != nil {
		return Package{}, err
	}

	command := exec.CommandContext( //nolint:gosec // compiler path comes from verified resolver
		ctx, compiler.Path, "list", "-mod=readonly", "-json", ".",
	)
	command.Dir = absolute
	command.Env = compiler.Env("", "", offline)

	var stdout, stderr bytes.Buffer

	command.Stdout = &stdout

	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return Package{}, fmt.Errorf("inspect workload package: %w: %s", err, stderr.String())
	}

	var metadata goListPackage
	if err := json.Unmarshal(stdout.Bytes(), &metadata); err != nil {
		return Package{}, fmt.Errorf("decode workload package: %w", err)
	}

	if metadata.Name == "main" {
		return Package{}, ErrMainPackage
	}

	if metadata.Module == nil || metadata.Module.Path == "" || metadata.Module.Dir == "" {
		return Package{}, ErrMissingModulePath
	}

	directory, err := filepath.EvalSymlinks(metadata.Dir)
	if err != nil {
		return Package{}, err
	}

	moduleRoot, err := filepath.EvalSymlinks(metadata.Module.Dir)
	if err != nil {
		return Package{}, err
	}

	return Package{
		ImportPath: metadata.ImportPath,
		Directory:  directory,
		ModulePath: metadata.Module.Path,
		ModuleRoot: moduleRoot,
	}, nil
}

// ModuleConfig returns selected module requirements and local replacements without changing source files.
func ModuleConfig(
	pkg Package,
) (requirements, replacements map[string]string, err error) {
	path := filepath.Join(pkg.ModuleRoot, "go.mod")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	file, err := modfile.Parse(path, data, nil)
	if err != nil {
		return nil, nil, err
	}

	requirements = make(map[string]string, len(file.Require)+1)
	for _, requirement := range file.Require {
		requirements[requirement.Mod.Path] = requirement.Mod.Version
	}

	if _, ok := requirements[pkg.ModulePath]; !ok {
		requirements[pkg.ModulePath] = moduleVersion(pkg.ModulePath)
	}

	replacements = map[string]string{pkg.ModulePath: pkg.ModuleRoot}

	for _, replacement := range file.Replace {
		if !modfile.IsDirectoryPath(replacement.New.Path) {
			continue
		}

		resolved, err := resolveLocalModule(pkg.ModuleRoot, replacement.New.Path)
		if err != nil {
			return nil, nil, err
		}

		replacements[replacement.Old.Path] = resolved
	}

	return requirements, replacements, nil
}

func resolveLocalModule(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(absolute)
}

func moduleVersion(path string) string {
	_, pathMajor, ok := module.SplitPathVersion(path)
	if !ok || pathMajor == "" {
		return "v0.0.0"
	}

	return strings.TrimPrefix(pathMajor, "/") + ".0.0"
}

func stroppyVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	if info.Main.Path == stroppyModulePath && validModuleVersion(info.Main.Version) {
		return info.Main.Version
	}

	for _, dependency := range info.Deps {
		if dependency.Path == stroppyModulePath && validModuleVersion(dependency.Version) {
			return dependency.Version
		}
	}

	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}

	revision := settings["vcs.revision"]
	if revision != "" && settings["vcs.modified"] == "true" {
		return revision + "+dirty"
	}

	return revision
}

func stroppyModuleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Path == stroppyModulePath && validModuleVersion(info.Main.Version) {
			return info.Main.Version
		}

		for _, dependency := range info.Deps {
			if dependency.Path == stroppyModulePath && validModuleVersion(dependency.Version) {
				return dependency.Version
			}
		}
	}

	return "v6.0.0"
}

func validModuleVersion(version string) bool {
	return version != "" && version != "(devel)" && strings.HasPrefix(version, "v") &&
		!strings.HasSuffix(version, "+dirty")
}
