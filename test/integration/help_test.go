//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolutionHelpListsQueryTimeout(t *testing.T) {
	repoRoot := findRepoRoot(t)
	binary := filepath.Join(repoRoot, "build", "stroppy")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("stroppy binary not found at %s (run `make build` first): %v", binary, err)
	}

	output, err := exec.Command(binary, "help", "resolution").CombinedOutput()
	if err != nil {
		t.Fatalf("stroppy help resolution: %v\n%s", err, output)
	}

	if !strings.Contains(string(output), "--query-timeout") {
		t.Fatalf("stroppy help resolution omitted --query-timeout:\n%s", output)
	}
}
