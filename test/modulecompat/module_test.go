package modulecompat_test

import (
	"archive/zip"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const (
	modulePath = "github.com/stroppy-io/stroppy/v6"
	version    = "v6.0.0-test"
)

func TestExternalModuleResolvesV6Release(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	proxyDir := t.TempDir()
	writeModuleProxy(t, proxyDir, repoRoot)

	consumerDir := t.TempDir()
	copyFile(t, filepath.Join("testdata", "consumer", "go.mod"), filepath.Join(consumerDir, "go.mod"))
	copyFile(t, filepath.Join("testdata", "consumer", "main.go.txt"), filepath.Join(consumerDir, "main.go"))

	proxyURL := (&url.URL{Scheme: "file", Path: proxyDir}).String()
	cmd := exec.Command("go", "build", "-mod=mod", "-o", filepath.Join(consumerDir, "consumer"), ".")
	cmd.Dir = consumerDir

	cmd.Env = append(os.Environ(), "GOPROXY="+proxyURL+",off", "GOSUMDB=off", "GOWORK=off")

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build external consumer: %v\n%s", err, output)
	}
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

	for _, name := range []string{"go.mod", "pkg/report/report.go"} {
		body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}

		entry, err := archive.Create(prefix + name)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
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
