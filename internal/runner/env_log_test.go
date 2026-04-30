package runner

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatScriptEnvEntries(t *testing.T) {
	got := formatScriptEnvEntries([]string{
		"LOAD_WORKERS=8",
		"PASSWORD=secret",
		"DATABASE_URL=postgres://user:pass@localhost/db",
		"API_KEY=secret",
	})

	require.Equal(t, []string{
		"API_KEY=<redacted>",
		"DATABASE_URL=<redacted>",
		"LOAD_WORKERS=8",
		"PASSWORD=<redacted>",
	}, got)
}

func TestUnknownScriptEnvKeys(t *testing.T) {
	probe := &Probeprint{
		Subprobe: Subprobe{
			EnvDeclarations: []EnvDeclaration{
				{Names: []string{"LOAD_WORKERS"}},
				{Names: []string{"SCALE_FACTOR", "WAREHOUSES"}},
			},
			Envs: []string{"LEGACY"},
		},
	}

	got := unknownScriptEnvKeys([]string{
		"LOAD_WORKERS=8",
		"WAREHOUSES=2",
		"LEGACY=yes",
		"TYPO=1",
	}, probe)

	require.Equal(t, []string{"TYPO"}, got)
}

func TestUnknownScriptEnvKeysSkipsValidationWithoutProbeData(t *testing.T) {
	require.Empty(t, unknownScriptEnvKeys([]string{"ANYTHING=1"}, nil))
	require.Empty(t, unknownScriptEnvKeys([]string{"ANYTHING=1"}, &Probeprint{}))
}

func TestKeepNewEnvEntries(t *testing.T) {
	known := map[string]struct{}{
		"FROM_OS":  {},
		"FROM_CLI": {},
	}

	kept, skipped := keepNewEnvEntries([]string{
		"FROM_CLI=cli",
		"FROM_FILE=file",
		"FROM_OS=file",
	}, known)

	require.Equal(t, []string{"FROM_FILE=file"}, kept)
	require.Equal(t, []string{"FROM_CLI", "FROM_OS"}, skipped)
}
