package view

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPoolDashboardKeepsNativeMetricsSeparate(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../docs/dashboard.json")
	require.NoError(t, err)

	var dashboard struct {
		Panels []struct {
			ID      int `json:"id"`
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	require.NoError(t, json.Unmarshal(data, &dashboard))

	ids := map[int]bool{}
	metrics := map[string]bool{}

	for _, panel := range dashboard.Panels {
		require.False(t, ids[panel.ID], "duplicate panel ID")

		ids[panel.ID] = true
		for _, target := range panel.Targets {
			if !strings.Contains(target.Expr, "${prefix}pool_") {
				continue
			}

			require.Contains(t, target.Expr, `instance=~"$pool_instance"`)
			require.Contains(t, target.Expr, `database=~"$pool_database"`)
			require.NotContains(t, target.Expr, "scenario=")
			require.NotContains(t, target.Expr, "sum(")
			metrics["${prefix}"+strings.Split(strings.TrimPrefix(target.Expr, "${prefix}"), "{")[0]] = true
		}
	}

	require.Contains(t, metrics, "${prefix}pool_sql_capacity_wait_seconds_total")
	require.Contains(t, metrics, "${prefix}pool_pgx_empty_acquire_seconds_total")
	require.Contains(t, metrics, "${prefix}pool_stats_available")
}
