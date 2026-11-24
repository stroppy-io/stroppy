//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/workloads/tpcds/tpcdsgen"
)

func TestTPCDSCallCenterSF1000(t *testing.T) {
	pool := NewTmpfsPG(t)
	ResetSchema(t, pool)
	runStroppy(t, time.Minute, "run", "tpcds", "-d", "pg",
		"-D", "url="+envOr(envTmpfsURL, defaultTmpfsURL), "--steps", "create_schema")

	db, err := driver.Dispatch(t.Context(), driver.Options{
		Config: &config.DriverConfig{
			DriverType: config.DriverTypePostgres,
			URL:        envOr(envTmpfsURL, defaultTmpfsURL),
		}, Logger: zap.NewNop(),
	})
	require.NoError(t, err)
	defer db.Teardown(context.Background())

	for _, workers := range []int{1, 32} {
		if _, err := pool.Exec(t.Context(), "TRUNCATE call_center"); err != nil {
			t.Fatal(err)
		}
		source, err := tpcdsgen.NewBatchSource("call_center", 1000)
		require.NoError(t, err)
		_, err = db.Insert(t.Context(), &driver.InsertRequest{
			Table: "call_center", Method: driver.InsertNative, Workers: workers, Source: source,
		})
		require.NoError(t, err, "workers=%d", workers)
		require.EqualValues(t, 42, CountRows(t, pool, "call_center"))
		AssertTableEquals(t, pool,
			"SELECT cc_sq_ft FROM call_center WHERE cc_call_center_sk IN (2,3,4) ORDER BY cc_call_center_sk",
			[]map[string]any{
				{"cc_sq_ft": int32(-1694677324)},
				{"cc_sq_ft": int32(444714690)},
				{"cc_sq_ft": int32(-835708383)},
			})
	}
}
