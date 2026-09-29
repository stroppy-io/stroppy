package workloadcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/stroppy-io/stroppy/v6/internal/toolchain"
)

var (
	ErrMainPackage       = errors.New("workload package must be importable, not package main")
	ErrMissingModulePath = errors.New("workload package has no module path")
)

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

	command := exec.CommandContext(ctx, compiler.Path, "list", "-json", ".")
	command.Dir = absolute
	command.Env = compiler.Env("", "", offline)

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return Package{}, fmt.Errorf("inspect workload package: %w: %s", err, stderr.String())
	}

	var metadata struct {
		ImportPath string
		Dir        string
		Name       string
		Module     *struct {
			Path string
			Dir  string
		}
	}
	if err := json.Unmarshal(stdout.Bytes(), &metadata); err != nil {
		return Package{}, fmt.Errorf("decode workload package: %w", err)
	}
	if metadata.Name == "main" {
		return Package{}, ErrMainPackage
	}
	if metadata.Module == nil || metadata.Module.Path == "" || metadata.Module.Dir == "" {
		return Package{}, ErrMissingModulePath
	}

	return Package{
		ImportPath: metadata.ImportPath,
		Directory:  metadata.Dir,
		ModulePath: metadata.Module.Path,
		ModuleRoot: metadata.Module.Dir,
	}, nil
}

// ModuleReplacements returns local replace directives declared by package module.
func ModuleReplacements(
	ctx context.Context,
	compiler *toolchain.Compiler,
	pkg Package,
	offline bool,
) (map[string]string, error) {
	command := exec.CommandContext(ctx, compiler.Path, "list", "-m", "-json", "all")
	command.Dir = pkg.ModuleRoot
	command.Env = compiler.Env("", "", offline)

	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("inspect workload module graph: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	replacements := map[string]string{pkg.ModulePath: pkg.ModuleRoot}

	for {
		var module struct {
			Path    string
			Replace *struct {
				Path string
				Dir  string
			}
		}

		if err := decoder.Decode(&module); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		if module.Replace == nil || module.Replace.Dir == "" {
			continue
		}

		path := module.Replace.Dir
		if !filepath.IsAbs(path) {
			path = filepath.Join(pkg.ModuleRoot, path)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}

		replacements[module.Path] = absolute
	}

	return replacements, nil
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
	return version != "" && version != "(devel)" && strings.HasPrefix(version, "v")
}
