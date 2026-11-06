package bench

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDriverIsolationPrecedence(t *testing.T) {
	t.Parallel()

	level := "serializable"
	definition := &Def{}
	definition.Drivers.declared = map[string]DriverConfig{
		"default": {Transactions: &TransactionConfig{DefaultIsolation: &level}},
	}
	reference := DriverRef{name: "default", owner: definition}
	require.Equal(t, IsoSerializable, reference.ResolveIsolation("", IsoReadCommitted))
	require.Equal(t, IsoRepeatableRead, reference.ResolveIsolation(IsoRepeatableRead, IsoReadCommitted))
	require.Equal(t, IsoDBDefault, reference.ResolveIsolation(IsoDBDefault, IsoReadCommitted))

	level = ""

	require.Equal(t, IsoReadCommitted, reference.ResolveIsolation("", IsoReadCommitted))
}
