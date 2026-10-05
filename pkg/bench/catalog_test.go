package bench

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogCopiesDescriptorIdentity(t *testing.T) {
	test := Test{Name: "example", Define: func(*Def) error { return nil }}
	catalog, err := NewCatalog(test)
	require.NoError(t, err)

	test.Name = "changed"
	descriptor, found := catalog.Test("example")
	require.True(t, found)
	require.Equal(t, "example", descriptor.Name)

	_, err = NewCatalog(test, test)
	require.Error(t, err)
}

func TestRegisteredDescriptorValidation(t *testing.T) { require.Panics(t, func() { Register(Test{}) }) }
