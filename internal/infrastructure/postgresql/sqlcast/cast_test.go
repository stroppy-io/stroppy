package sqlcast //nolint:testpackage // test package

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestPgTextToStr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected string
		input    pgtype.Text
	}{
		{
			"Valid text",
			"Hello",
			pgtype.Text{String: "Hello", Valid: true},
		},
		{
			"Invalid text",
			"",
			pgtype.Text{Valid: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := PgTextToStr(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestStrToPgText(t *testing.T) {
	t.Parallel()

	result := StrToPgText("Hello")
	require.Equal(t, pgtype.Text{String: "Hello", Valid: true}, result)
}

func TestPgTextToStrPtr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected *string
		name     string
		input    pgtype.Text
	}{
		{
			func() *string {
				str := "Hello"

				return &str
			}(),
			"Valid text",
			pgtype.Text{String: "Hello", Valid: true},
		},
		{
			nil,
			"Invalid text",
			pgtype.Text{Valid: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := PgTextToStrPtr(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestStrPtrToPgText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    *string
		expected pgtype.Text
	}{
		{
			"Valid string pointer",
			func() *string {
				str := "Hello"

				return &str
			}(),
			pgtype.Text{String: "Hello", Valid: true},
		},
		{
			"Nil string pointer",
			nil,
			pgtype.Text{Valid: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := StrPrtToPgText(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPgUUIDToUUID(t *testing.T) {
	t.Parallel()

	validUUID := uuid.New()
	tests := []struct {
		name     string
		input    pgtype.UUID
		expected uuid.UUID
	}{
		{
			"Valid UUID",
			pgtype.UUID{Bytes: validUUID, Valid: true},
			validUUID,
		},
		{
			"Invalid UUID",
			pgtype.UUID{Valid: false},
			uuid.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := PgUUIDToUUID(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestUUIDToPgUUID(t *testing.T) {
	t.Parallel()

	validUUID := uuid.New()
	result := UUIDToPgUUID(validUUID)
	require.Equal(t, pgtype.UUID{Bytes: validUUID, Valid: true}, result)
}

func TestPgUUIDToUUIDPtr(t *testing.T) {
	t.Parallel()

	validUUID := uuid.New()
	tests := []struct {
		expected *uuid.UUID
		name     string
		input    pgtype.UUID
	}{
		{
			&validUUID,
			"Valid UUID",
			pgtype.UUID{Bytes: validUUID, Valid: true},
		},
		{
			nil,
			"Invalid UUID",
			pgtype.UUID{Valid: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := PgUUIDToUUIDPtr(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestUUIDPtrToPgUUID(t *testing.T) {
	t.Parallel()

	validUUID := uuid.New()
	tests := []struct {
		input    *uuid.UUID
		name     string
		expected pgtype.UUID
	}{
		{
			&validUUID,
			"Valid UUID pointer",
			pgtype.UUID{Bytes: validUUID, Valid: true},
		},
		{
			nil,
			"Nil UUID pointer",
			pgtype.UUID{Valid: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := UUIDPtrToPgUUID(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPgTimestamptzToTime(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		expected time.Time
		input    pgtype.Timestamptz
		name     string
	}{
		{
			now,
			pgtype.Timestamptz{Time: now, Valid: true},
			"Valid timestamptz",
		},
		{
			time.Time{},
			pgtype.Timestamptz{Valid: false},
			"Invalid timestamptz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := PgTimestamptzToTime(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestTimeToPgTimestamptz(t *testing.T) {
	t.Parallel()

	now := time.Now()
	result := TimeToPgTimestamptz(now)

	require.Equal(t, pgtype.Timestamptz{Time: now, Valid: true}, result)
}
