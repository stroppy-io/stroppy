package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestJWTManager_GenerateToken(t *testing.T) {
	jwtManager := NewJWTManager("test-secret", time.Hour)

	token, err := jwtManager.GenerateToken(1, "testuser")
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestJWTManager_ValidateToken(t *testing.T) {
	jwtManager := NewJWTManager("test-secret", time.Hour)

	// Generate a valid token
	token, err := jwtManager.GenerateToken(1, "testuser")
	assert.NoError(t, err)

	// Validate the token
	claims, err := jwtManager.ValidateToken(token)
	assert.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, 1, claims.UserID)
	assert.Equal(t, "testuser", claims.Username)
}

func TestJWTManager_ValidateToken_Invalid(t *testing.T) {
	jwtManager := NewJWTManager("test-secret", time.Hour)

	// Test with invalid token
	claims, err := jwtManager.ValidateToken("invalid-token")
	assert.Error(t, err)
	assert.Nil(t, claims)
	assert.Equal(t, ErrInvalidToken, err)
}

func TestJWTManager_ValidateToken_Expired(t *testing.T) {
	jwtManager := NewJWTManager("test-secret", -time.Hour) // Expired token

	token, err := jwtManager.GenerateToken(1, "testuser")
	assert.NoError(t, err)

	// Try to validate expired token
	claims, err := jwtManager.ValidateToken(token)
	assert.Error(t, err)
	assert.Nil(t, claims)
	assert.Equal(t, ErrTokenExpired, err)
}

func TestHashPassword(t *testing.T) {
	password := "testpassword"
	hash, err := HashPassword(password)

	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)
}

func TestCheckPassword(t *testing.T) {
	password := "testpassword"
	hash, err := HashPassword(password)
	assert.NoError(t, err)

	// Test correct password
	assert.True(t, CheckPassword(password, hash))

	// Test incorrect password
	assert.False(t, CheckPassword("wrongpassword", hash))
}
