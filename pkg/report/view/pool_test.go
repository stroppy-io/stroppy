package view

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPoolViewPreservesCoverageAndSemantics(t *testing.T) {
	t.Parallel()

	p, err := reportPage([]byte(`{"schema":4,"kind":"run","workload":"test","pools":[
{"step":"workload","database":"primary","driver":"mysql","backend":"database/sql","scope":"SQL only",
"supported":true,"open":2,"in_use":0,"idle":2,"max_connections":0,"wait_count":3,"wait_seconds":1.5},
{"step":"workload","database":"other","driver":"picodata","supported":false,"reason":"SDK stats unavailable"}
]}`), "")
	require.NoError(t, err)
	require.Len(t, p.Pools, 2)
	require.Equal(t, "Unlimited", p.Pools[0].Maximum)
	require.Empty(t, p.Pools[0].Reason)
	require.Contains(t, p.Pools[0].Semantics, "includes canceled waits")
	require.Equal(t, "Unavailable", p.Pools[1].Status)
	require.Empty(t, p.Pools[1].Seconds)

	stats := metricStats(map[string]any{"type": "gauge", "series": []any{1, 1}}, "")
	require.Equal(t, "Per series", stats[0].Text)

	var output bytes.Buffer
	require.NoError(t, templates.ExecuteTemplate(&output, "report", p))
	require.Contains(t, output.String(), "Connection pools")
	require.Contains(t, output.String(), "Unavailable does not mean zero wait")
	require.Contains(t, output.String(), "SDK stats unavailable")
}
