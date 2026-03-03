package probe

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/internal/runner"
)

// Script at scriptPath probed with workdir assumed.
// Required to probe in a full stroppy workdir with all the scripts.
// Common case is if you generated workdir with "stroppy gen --workdir=<dir> --preset=<presed>".
func Script(scriptPath string) (*runner.Probeprint, error) {
	return runner.ProbeScript(scriptPath)
}

// ScriptInTmp works as [Script], but don't requires working directory.
// sqlPath might be empty "".
func ScriptInTmp(scriptPath, sqlPath string) (*runner.Probeprint, error) {
	tempDir, _, err := runner.CreateAndInitTempDir(zap.NewNop(), scriptPath, sqlPath)
	if err != nil {
		return nil, fmt.Errorf("error while creating temporary dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	return runner.ProbeScript(filepath.Join(tempDir, filepath.Base(scriptPath)))
}
