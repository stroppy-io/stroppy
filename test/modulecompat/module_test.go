package modulecompat_test

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/internal/author"
	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	_ "github.com/stroppy-io/stroppy/v6/workloads/all"
)

const (
	modulePath = "github.com/stroppy-io/stroppy/v6"
	version    = "v6.0.0-test"
)

func TestExternalModuleResolvesV6Release(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	proxyDir := t.TempDir()
	writeModuleProxy(t, proxyDir, repoRoot)

	consumerDir := prepareConsumer(t)
	binary := filepath.Join(consumerDir, "consumer")
	proxyURL := (&url.URL{Scheme: "file", Path: proxyDir}).String()
	moduleCache := filepath.Join(t.TempDir(), "modcache")
	t.Cleanup(func() { makeWritable(t, moduleCache) })
	buildEnv := append(os.Environ(),
		"GOPROXY="+proxyURL+",https://proxy.golang.org,direct",
		"GONOSUMDB="+modulePath,
		"GOWORK=off",
		"GOMODCACHE="+moduleCache,
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
	)

	runCommand(t, consumerDir, buildEnv, "go", "test", "-mod=mod", "-race", "./...")
	runCommand(t, consumerDir, buildEnv, "go", "build", "-mod=mod", "-o", binary, ".")

	home := t.TempDir()
	runEnv := append(os.Environ(), "HOME="+home)

	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, "external/fixture"},
		{[]string{"probe", "-o", "json"}, `"external/fixture"`},
		{[]string{"version", "--json"}, `"stroppy"`},
		{[]string{"-d", "noop", "--iterations", "2", "--report-format", "json"}, `"workload": "external/fixture"`},
	} {
		output := runCommand(t, consumerDir, runEnv, binary, test.args...)
		if !strings.Contains(output, test.want) {
			t.Fatalf("%s output = %q, want %q", test.args, output, test.want)
		}
	}

	history, err := filepath.Glob(filepath.Join(home, ".stroppy", "reports", "*.json"))
	if err != nil || len(history) != 1 {
		t.Fatalf("report history = %v, error = %v", history, err)
	}
}

func TestPublishedProjectsCompileAsExternalModules(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	proxyDir := t.TempDir()
	writeModuleProxy(t, proxyDir, repoRoot)
	proxyURL := (&url.URL{Scheme: "file", Path: proxyDir}).String()
	moduleCache := filepath.Join(t.TempDir(), "modcache")
	t.Cleanup(func() { makeWritable(t, moduleCache) })
	env := append(os.Environ(), "GOPROXY="+proxyURL+",https://proxy.golang.org,direct", "GONOSUMDB="+modulePath,
		"GOWORK=off", "GOMODCACHE="+moduleCache, "GOCACHE="+filepath.Join(t.TempDir(), "gocache"))

	descriptions, err := bench.DescribeAll()
	if err != nil {
		t.Fatal(err)
	}

	checks := make([]struct {
		name  string
		files map[string][]byte
	}, 0, 1+len(descriptions))

	starter, err := author.Project("starter", "example.com/starter", version, author.Starter("starter"))
	if err != nil {
		t.Fatal(err)
	}

	checks = append(checks, struct {
		name  string
		files map[string][]byte
	}{"starter", starter})

	for _, description := range descriptions {
		test, _ := bench.Lookup(description.Name)

		published, err := author.Read(test.Source)
		if err != nil {
			t.Fatal(err)
		}

		files, err := author.Project(description.Name, "example.com/fork", version, published)
		if err != nil {
			t.Fatal(err)
		}

		checks = append(checks, struct {
			name  string
			files map[string][]byte
		}{description.Name, files})
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "project")
			if err := author.Write(directory, check.files); err != nil {
				t.Fatal(err)
			}

			runCommand(t, directory, env, "go", "list", "-mod=mod", "-deps", "-test", "./...")
			runCommand(t, directory, env, "go", "test", "-short", "./...")
			runCommand(t, directory, env, "go", "build", "-o", filepath.Join(directory, "workload-runner"), ".")

			if check.name == "starter" {
				runCommand(t, directory, env, "go", "test", "-race", "./...")
				runner := filepath.Join(directory, "workload-runner")
				runCommand(t, directory, env, runner, "--iterations", "2", "--no-report")
				fork := filepath.Join(t.TempDir(), "restored")
				runCommand(t, directory, env, runner, "eject", "starter", fork)
				runCommand(t, fork, env, "go", "test", "-race", "./...")
				runCommand(t, fork, env, "go", "run", ".", "--iterations", "2", "--no-report")
				initialized := filepath.Join(t.TempDir(), "initialized")
				runCommand(t, directory, env, runner, "init", initialized)
				runCommand(t, initialized, env, "go", "test", "-race", "./...")
				runCommand(t, initialized, env, "go", "run", ".", "--iterations", "2", "--no-report")

				installed := filepath.Join(directory, "stroppy")
				runCommand(t, directory, env, "go", "build", "-mod=mod", "-o", installed, modulePath+"/cmd/stroppy")
				managedEnv := append(env, "HOME="+t.TempDir())
				runCommand(t, directory, managedEnv, installed, "build", initialized)
				runCommand(t, directory, managedEnv, installed, "run", "initialized", "--iterations", "2", "--no-report")
				portable := filepath.Join(directory, "portable")
				runCommand(t, directory, managedEnv, installed, "export", "initialized", "-o", portable)
				restored := filepath.Join(t.TempDir(), "exported-fork")
				runCommand(t, directory, managedEnv, portable, "eject", "initialized", restored)
				runCommand(t, restored, env, "go", "test", "-race", "./...")
				runCommand(t, restored, env, "go", "run", ".", "--iterations", "2", "--no-report")
			}
		})
	}
}

func prepareConsumer(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	copyFile(t, filepath.Join("testdata", "consumer", "go.mod"), filepath.Join(dir, "go.mod"))
	copyFile(t, filepath.Join("testdata", "consumer", "main.go.txt"), filepath.Join(dir, "main.go"))
	copyFile(t, filepath.Join("testdata", "consumer", "author_api_test.go.txt"), filepath.Join(dir, "author_api_test.go"))

	examples := filepath.Join(dir, "authoring")
	if err := os.Mkdir(examples, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"query.go", "accounts.go", "mirror.go"} {
		copyFile(t, filepath.Join("..", "..", "examples", "authoring", name), filepath.Join(examples, name))
	}

	return dir
}

func runCommand(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}

	return string(output)
}

func writeModuleProxy(t *testing.T, proxyDir, repoRoot string) {
	t.Helper()

	versionDir := filepath.Join(proxyDir, filepath.FromSlash(modulePath), "@v")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	copyFile(t, filepath.Join(repoRoot, "go.mod"), filepath.Join(versionDir, version+".mod"))
	writeFile(t, filepath.Join(versionDir, "list"), []byte(version+"\n"))
	writeFile(t, filepath.Join(versionDir, version+".info"), fmt.Appendf(
		nil,
		`{"Version":%q,"Time":"2026-01-01T00:00:00Z"}`,
		version,
	))

	file, err := os.Create(filepath.Join(versionDir, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}

	archive := zip.NewWriter(file)
	prefix := modulePath + "@" + version + "/"
	archived := 0

	err = filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			name := entry.Name()
			if path != repoRoot && (strings.HasPrefix(name, ".") || name == "build" || name == "bin") {
				return filepath.SkipDir
			}

			return nil
		}

		relative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}

		if !moduleArchiveFile(relative) {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		writer, err := archive.Create(prefix + filepath.ToSlash(relative))
		if err != nil {
			return err
		}

		_, err = writer.Write(body)
		archived++

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if archived == 0 {
		t.Fatal("module archive is empty")
	}

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func moduleArchiveFile(path string) bool {
	if path == "go.mod" || path == "go.sum" || path == "LICENSE" ||
		path == "internal/author/LICENSE" || path == "internal/pgnoop/release.sha256" {
		return true
	}

	if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
		return true
	}

	for _, prefix := range []string{"third_party/", "workloads/"} {
		if strings.HasPrefix(filepath.ToSlash(path), prefix) {
			return true
		}
	}

	return false
}

func makeWritable(t *testing.T, root string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}

		return os.Chmod(path, 0o600)
	})

	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("make module cache writable: %v", err)
	}
}

func copyFile(t *testing.T, source, target string) {
	t.Helper()

	body, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, target, body)
}

func writeFile(t *testing.T, path string, body []byte) {
	t.Helper()

	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
