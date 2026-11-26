package view

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgressRetainsNativeSourcesAndHeuristicLabel(t *testing.T) {
	rendered, err := reportPage([]byte(`{"kind":"run","schema":1,"workload":"queries","progress":{
		"ratio":0.375,"status":"canceled","method":"equal selected phase weights",
		"sources":[{"step":"workload","worker":1,"activity":"query_8","unit":"queries","current":3,"total":8}]
	}}`), "")
	require.NoError(t, err)
	require.Equal(t, "37.5%", rendered.ProgressPercent)
	require.NotEmpty(t, rendered.Progress.Fields)
}
