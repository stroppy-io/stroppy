package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRun(t *testing.T) {
	tests := []struct {
		name        string
		userID      int
		runName     string
		description string
		config      string
		expectError bool
		expectedErr error
	}{
		{
			name:        "valid run",
			userID:      1,
			runName:     "test run",
			description: "test description",
			config:      `{"key": "value"}`,
			expectError: false,
		},
		{
			name:        "invalid user id",
			userID:      0,
			runName:     "test run",
			description: "test description",
			config:      `{"key": "value"}`,
			expectError: true,
			expectedErr: ErrInvalidRunData,
		},
		{
			name:        "empty name",
			userID:      1,
			runName:     "",
			description: "test description",
			config:      `{"key": "value"}`,
			expectError: true,
			expectedErr: ErrInvalidRunData,
		},
		{
			name:        "empty config",
			userID:      1,
			runName:     "test run",
			description: "test description",
			config:      "",
			expectError: true,
			expectedErr: ErrInvalidRunData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, err := NewRun(tt.userID, tt.runName, tt.description, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr, err)
				assert.Nil(t, run)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, run)
				assert.Equal(t, tt.userID, run.UserID)
				assert.Equal(t, tt.runName, run.Name)
				assert.Equal(t, tt.description, run.Description)
				assert.Equal(t, tt.config, run.Config)
				assert.Equal(t, StatusPending, run.Status)
				assert.False(t, run.CreatedAt.IsZero())
				assert.False(t, run.UpdatedAt.IsZero())
			}
		})
	}
}

func TestRun_UpdateStatus(t *testing.T) {
	run, err := NewRun(1, "test run", "description", `{"key": "value"}`)
	assert.NoError(t, err)

	// Test running status
	run.UpdateStatus(StatusRunning)
	assert.Equal(t, StatusRunning, run.Status)
	assert.NotNil(t, run.StartedAt)
	assert.Nil(t, run.CompletedAt)

	// Test completed status
	run.UpdateStatus(StatusCompleted)
	assert.Equal(t, StatusCompleted, run.Status)
	assert.NotNil(t, run.StartedAt)
	assert.NotNil(t, run.CompletedAt)
}

func TestRun_IsOwnedBy(t *testing.T) {
	run, err := NewRun(1, "test run", "description", `{"key": "value"}`)
	assert.NoError(t, err)

	assert.True(t, run.IsOwnedBy(1))
	assert.False(t, run.IsOwnedBy(2))
}
