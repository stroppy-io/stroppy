package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRun(t *testing.T) {
	tests := []struct {
		name        string
		runName     string
		description string
		config      string
		expectError bool
		expectedErr error
	}{
		{
			name:        "valid run",
			runName:     "test run",
			description: "test description",
			config:      `{"key": "value"}`,
			expectError: false,
		},
		{
			name:        "empty name",
			runName:     "",
			description: "test description",
			config:      `{"key": "value"}`,
			expectError: true,
			expectedErr: ErrInvalidRunData,
		},
		{
			name:        "empty config",
			runName:     "test run",
			description: "test description",
			config:      "",
			expectError: true,
			expectedErr: ErrInvalidRunData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, err := NewRun(tt.runName, tt.description, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr, err)
				assert.Nil(t, run)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, run)
				assert.Equal(t, tt.runName, run.Name)
				assert.Equal(t, tt.description, run.Description)
				assert.Equal(t, tt.config, run.Config)
				assert.Equal(t, StatusPending, run.Status)
				assert.False(t, run.CreatedAt.IsZero())
				assert.False(t, run.UpdatedAt.IsZero())
				// Проверяем, что TPS метрики инициализированы
				assert.NotNil(t, run.TPSMetrics)
			}
		})
	}
}

func TestRun_UpdateStatus(t *testing.T) {
	run, err := NewRun("test run", "description", `{"key": "value"}`)
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

func TestRun_UpdateTPSMetrics(t *testing.T) {
	run, err := NewRun("test run", "description", `{"key": "value"}`)
	assert.NoError(t, err)

	// Test updating TPS metrics
	max := 100.5
	min := 50.0
	average := 75.25
	p95 := 95.0
	p99 := 99.0

	metrics := TPSMetrics{
		Max:     &max,
		Min:     &min,
		Average: &average,
		P95:     &p95,
		P99:     &p99,
	}

	run.UpdateTPSMetrics(metrics)
	assert.Equal(t, &max, run.TPSMetrics.Max)
	assert.Equal(t, &min, run.TPSMetrics.Min)
	assert.Equal(t, &average, run.TPSMetrics.Average)
	assert.Equal(t, &p95, run.TPSMetrics.P95)
	assert.Equal(t, &p99, run.TPSMetrics.P99)
}
