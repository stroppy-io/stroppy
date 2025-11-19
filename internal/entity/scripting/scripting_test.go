package scripting

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

func TestBuildBase64Script(t *testing.T) {
	data, err := BuildBase64Script(&crossplane.Deployment_Strategy_Scripting{
		Cmd:     "echo 'Hello, World!'",
		Workdir: ".",
		FilesToWrite: []*crossplane.FsFile{
			{
				Path:    "stroppy.ts",
				Content: []byte("console.log('Hello, World!');"),
			},
			{
				Path:    "stroppy.sh",
				Content: []byte("echo 'Hello, World!'"),
			},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, data)

	t.Log(string(data))
}
