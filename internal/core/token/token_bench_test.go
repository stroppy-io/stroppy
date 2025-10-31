package token

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkTokenValidator_ValidateAccessToken(b *testing.B) {
	testConfig := NewTestTokenConfig()
	adds := map[string]interface{}{
		"some_data": "data",
	}
	b.SetBytes(2)
	creator := NewTokenActor(testConfig)
	pair, err := creator.NewAccessToken(testUserID, adds)
	if err != nil {
		b.Fatal(err)
	}
	require.NoError(b, err)
	b.ReportAllocs()
	validator := NewTokenActor(testConfig)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = validator.ValidateToken(pair)
		require.NoError(b, err)
	}
}
