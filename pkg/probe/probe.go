package probe

import (
	"fmt"
	"os"

	"github.com/stroppy-io/stroppy/internal/runner"
	"go.uber.org/zap"
)

func ProbeScript(scriptPath string) (*runner.Probeprint, error) {
	return runner.ProbeScript(scriptPath)
}

func ProbeScriptInTmp(scriptPath string, sqlPath string) (*runner.Probeprint, error) {
	tempDir, err := runner.CreateAndInitTempDir(zap.NewNop(), scriptPath, sqlPath)
	if err != nil {
		return nil, fmt.Errorf("error while creating temporary dir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	return runner.ProbeScript(scriptPath)
}
