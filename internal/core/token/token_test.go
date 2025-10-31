package token

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testSecret = "94a7fbe7c18c9756c79ef9d87bf28d22ab94f2be471be024b47564daa66a70002de28978cb73e76328e3ba12d7706da16a06bf4d1ef41acdede3a5d1052993d2"
	testUserID = "01JACX15TVHSGKN66AVVSKKZ2M"
)

const (
	DefaultAccessExpireTime  = 1 * time.Hour
	DefaultRefreshExpireTime = 30 * 24 * time.Hour
	DefaultLeewaySeconds     = 30
	DefaultIssuer            = "komeet-auth"
)

func NewTestTokenConfig() *Config {
	return &Config{
		HmacSecretKey: testSecret,
		LeewaySeconds: DefaultLeewaySeconds,
		Issuer:        DefaultIssuer,
		AccessExpire:  DefaultAccessExpireTime,
		RefreshExpire: DefaultRefreshExpireTime,
	}
}

func NewTestTokenActor() *Actor {
	return NewTokenActor(NewTestTokenConfig())
}

func TestGenerateTokenPair(t *testing.T) {
	pair, err := NewTestTokenActor().NewAccessToken(testUserID, map[string]interface{}{})
	require.NoError(t, err)
	require.NotEmpty(t, pair)
	t.Log(pair)
}
