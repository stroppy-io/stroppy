package record

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotOwnsValuesAndKeepsCrossDatabaseOrder(t *testing.T) {
	recorder := &Recorder{}
	value, err := Encode([]byte("original"))
	require.NoError(t, err)

	first := Operation{Scope: Scope{Step: "work"}, Kind: "query", Arguments: map[string]Value{"bytes": value}}
	recorder.Add(first)
	first.Arguments["bytes"] = Value{Type: "changed"}

	recorder.Add(Operation{Scope: Scope{Step: "work", Database: "secondary"}, Kind: "query"})
	recorder.Add(Operation{Scope: Scope{Step: "work"}, Kind: "commit"})
	snapshot := recorder.Operations()
	require.Equal(t, "[]uint8", snapshot[0].Arguments["bytes"].Type)
	snapshot[0].Arguments["bytes"] = Value{Type: "mutated"}
	require.Equal(t, "[]uint8", recorder.Operations()[0].Arguments["bytes"].Type)
	require.Equal(t, "secondary", snapshot[1].Scope.Database)
	require.Equal(t, "commit", snapshot[2].Kind)
}

func TestConcurrentRecordingHasStableStreams(t *testing.T) {
	recorder := &Recorder{}

	var workers sync.WaitGroup
	for worker := range 4 {
		workers.Go(func() {
			for iteration := range 10 {
				recorder.Add(Operation{Scope: Scope{Step: "work", Worker: worker, Iteration: uint64(iteration)}, Kind: "query"})
			}
		})
	}

	workers.Wait()

	var first, second bytes.Buffer
	require.NoError(t, recorder.WriteTo(&first))
	require.NoError(t, recorder.WriteTo(&second))
	require.Equal(t, first.String(), second.String())
	require.True(t, json.Valid(first.Bytes()))
	require.Len(t, recorder.Operations(), 40)
}
