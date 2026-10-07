package recording

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

func TestFileRecordingRefusesOverwriteAndRecordsTransactions(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "recording.json")
	backend, err := newDriver(t.Context(), driver.Options{Config: &config.DriverConfig{URL: filename}})
	require.NoError(t, err)
	_, err = newDriver(t.Context(), driver.Options{Config: &config.DriverConfig{URL: filename}})
	require.ErrorIs(t, err, os.ErrExist)
	ctx := record.WithScope(t.Context(), record.Scope{Database: "default", Step: "work"})
	tx, err := backend.Begin(ctx, config.TxIsolationLevelSerializable)
	require.NoError(t, err)
	result, err := tx.RunQuery(ctx, "SELECT :value", map[string]any{"value": int64(1)})
	require.NoError(t, err)
	require.False(t, result.Rows.Next())
	require.NoError(t, result.Rows.Close())
	require.NoError(t, tx.Rollback(ctx))
	require.NoError(t, backend.Teardown(ctx))

	data, err := os.ReadFile(filename)
	require.NoError(t, err)

	var document struct {
		Schema     int                `json:"schema"`
		Operations []record.Operation `json:"operations"`
	}
	require.NoError(t, json.Unmarshal(data, &document))
	require.Equal(t, 1, document.Schema)
	require.Len(t, document.Operations, 3)
	require.Equal(t, "begin", document.Operations[0].Kind)
	require.Equal(t, "rollback", document.Operations[2].Kind)
}

func TestExplicitResponseErrorAndCancellation(t *testing.T) {
	recorder := &record.Recorder{}
	sentinel := errors.New("query failure")
	recorder.Reply("SELECT", record.Response{Err: sentinel})
	backend, err := newDriver(t.Context(), driver.Options{Config: &config.DriverConfig{Recording: recorder}})
	require.NoError(t, err)
	_, err = backend.RunQuery(t.Context(), "SELECT", nil)
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, "query failure", recorder.Operations()[0].Error)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = backend.RunQuery(ctx, "SELECT", nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, recorder.Operations(), 1)
}
