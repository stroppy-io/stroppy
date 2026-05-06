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

// ScriptWithEnv works as Script with a pre-populated mocked __ENV object.
func ScriptWithEnv(scriptPath string, env map[string]string) (*runner.Probeprint, error) {
	return runner.ProbeScriptWithEnv(scriptPath, env)
}

// ScriptInTmp works as [Script], but don't requires working directory.
// sqlPath might be empty "".
func ScriptInTmp(scriptPath, sqlPath string) (*runner.Probeprint, error) {
	return ScriptInTmpWithEnv(scriptPath, sqlPath, nil)
}

// ScriptInTmpWithEnv works as ScriptInTmp with a pre-populated mocked __ENV object.
func ScriptInTmpWithEnv(scriptPath, sqlPath string, env map[string]string) (*runner.Probeprint, error) {
	input, err := runner.ResolveInput(scriptPath, sqlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve input: %w", err)
	}

	tempDir, _, err := runner.CreateAndInitTempDir(zap.NewNop(), input)
	if err != nil {
		return nil, fmt.Errorf("error while creating temporary dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	return runner.ProbeScriptWithEnv(filepath.Join(tempDir, input.Script.Name), env)
}
