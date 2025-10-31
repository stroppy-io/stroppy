package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewUser(t *testing.T) {
	tests := []struct {
		name         string
		username     string
		passwordHash string
		expectError  bool
		expectedErr  error
	}{
		{
			name:         "valid user",
			username:     "testuser",
			passwordHash: "hashed_password",
			expectError:  false,
		},
		{
			name:         "empty username",
			username:     "",
			passwordHash: "hashed_password",
			expectError:  true,
			expectedErr:  ErrInvalidUserData,
		},
		{
			name:         "empty password hash",
			username:     "testuser",
			passwordHash: "",
			expectError:  true,
			expectedErr:  ErrInvalidUserData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := NewUser(tt.username, tt.passwordHash)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr, err)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.username, user.Username)
				assert.Equal(t, tt.passwordHash, user.PasswordHash)
				assert.False(t, user.CreatedAt.IsZero())
				assert.False(t, user.UpdatedAt.IsZero())
			}
		})
	}
}
