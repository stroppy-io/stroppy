package workloadcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestModuleConfigPreservesReplacements(t *testing.T) {
	moduleRoot := filepath.Join(t.TempDir(), "module root")

	localRoot := filepath.Join(t.TempDir(), "local replacement")
	if err := os.MkdirAll(moduleRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(localRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	file := new(modfile.File)
	if err := file.AddModuleStmt("example.com/workload"); err != nil {
		t.Fatal(err)
	}

	if err := file.AddGoStmt("1.27"); err != nil {
		t.Fatal(err)
	}

	if err := file.AddRequire("example.com/dependency", "v1.0.0"); err != nil {
		t.Fatal(err)
	}

	if err := file.AddReplace("example.com/local", "", localRoot, ""); err != nil {
		t.Fatal(err)
	}

	if err := file.AddReplace("example.com/dependency", "v1.0.0", "example.com/fork", "v1.2.3"); err != nil {
		t.Fatal(err)
	}

	goMod, err := file.Format()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), goMod, 0o600); err != nil {
		t.Fatal(err)
	}

	_, replacements, err := ModuleConfig(&Package{
		ModulePath: "example.com/workload", ModuleRoot: moduleRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolvedLocalRoot, err := filepath.EvalSymlinks(localRoot)
	if err != nil {
		t.Fatal(err)
	}

	want := moduleReplacements{
		{Path: "example.com/workload"}: {Path: moduleRoot},
		{Path: "example.com/local"}:    {Path: resolvedLocalRoot},
		{Path: "example.com/dependency", Version: "v1.0.0"}: {
			Path: "example.com/fork", Version: "v1.2.3",
		},
	}
	for oldModule, newModule := range want {
		if got := replacements[oldModule]; got != newModule {
			t.Errorf("replacement %s = %s, want %s", oldModule, got, newModule)
		}
	}
}

func TestWriteRunnerModuleQuotesReplacementPaths(t *testing.T) {
	directory := t.TempDir()

	pathWithSpace := filepath.Join(t.TempDir(), "module root")
	if err := os.MkdirAll(pathWithSpace, 0o700); err != nil {
		t.Fatal(err)
	}

	err := writeRunnerModule(
		directory,
		[]string{"example.com/workload"},
		map[string]string{stroppyModulePath: "v6.0.0", "example.com/workload": "v0.0.0"},
		moduleReplacements{
			{Path: "example.com/workload"}: {Path: pathWithSpace},
			{Path: "example.com/dependency", Version: "v1.0.0"}: {
				Path: "example.com/fork", Version: "v1.2.3",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	file, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		t.Fatalf("parse generated go.mod: %v\n%s", err, data)
	}

	if len(file.Replace) != 2 {
		t.Fatalf("generated replacements = %#v", file.Replace)
	}

	if file.Replace[0].New.Path != "example.com/fork" || file.Replace[0].New.Version != "v1.2.3" {
		t.Fatalf("version replacement = %#v", file.Replace[0])
	}

	if file.Replace[1].New.Path != pathWithSpace || file.Replace[1].New.Version != "" {
		t.Fatalf("directory replacement = %#v", file.Replace[1])
	}
}
