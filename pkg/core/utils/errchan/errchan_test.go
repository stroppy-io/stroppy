package errchan

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChanResult_IsError(t *testing.T) {
	tests := []struct {
		name     string
		result   *ChanResult[string]
		expected bool
	}{
		{
			name: "no error",
			result: &ChanResult[string]{
				data:  stringPtr("test"),
				Error: nil,
			},
			expected: false,
		},
		{
			name: "with error",
			result: &ChanResult[string]{
				data:  nil,
				Error: errors.New("test error"),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.result.IsError()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestChanResult_Unwrap(t *testing.T) {
	tests := []struct {
		name        string
		result      *ChanResult[string]
		expectPanic bool
		expected    *string
	}{
		{
			name: "successful unwrap",
			result: &ChanResult[string]{
				data:  stringPtr("test"),
				Error: nil,
			},
			expectPanic: false,
			expected:    stringPtr("test"),
		},
		{
			name: "panic on error",
			result: &ChanResult[string]{
				data:  nil,
				Error: errors.New("test error"),
			},
			expectPanic: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectPanic {
				require.Panics(t, func() {
					tt.result.Unwrap()
				})
			} else {
				result := tt.result.Unwrap()
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestChanResult_Get(t *testing.T) {
	tests := []struct {
		name          string
		result        *ChanResult[string]
		expectedData  *string
		expectedError error
	}{
		{
			name: "successful get",
			result: &ChanResult[string]{
				data:  stringPtr("test"),
				Error: nil,
			},
			expectedData:  stringPtr("test"),
			expectedError: nil,
		},
		{
			name: "get with error",
			result: &ChanResult[string]{
				data:  nil,
				Error: errors.New("test error"),
			},
			expectedData:  nil,
			expectedError: errors.New("test error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.result.Get()
			require.Equal(t, tt.expectedData, data)

			if tt.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tt.expectedError.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSend(t *testing.T) {
	ch := make(Chan[string], 1)

	Send(ch, stringPtr("test"), nil)

	result := <-ch
	require.NotNil(t, result)
	require.Equal(t, "test", *result.data)
	require.NoError(t, result.Error)
}

func TestSend_WithError(t *testing.T) {
	ch := make(Chan[string], 1)
	testError := errors.New("test error")

	Send[string](ch, nil, testError)

	result := <-ch
	require.NotNil(t, result)
	require.Nil(t, result.data)
	require.Error(t, result.Error)
	require.Equal(t, testError.Error(), result.Error.Error())
}

func TestClose(t *testing.T) {
	ch := make(Chan[string], 1)

	Close[string](ch)

	_, ok := <-ch
	require.False(t, ok)
}

func TestReceive(t *testing.T) {
	ch := make(Chan[string], 1)
	Send(ch, stringPtr("test"), nil)

	data, err := Receive[string](ch)
	require.NoError(t, err)
	require.Equal(t, "test", *data)
}

func TestReceive_WithError(t *testing.T) {
	ch := make(Chan[string], 1)
	testError := errors.New("test error")
	Send[string](ch, nil, testError)

	data, err := Receive[string](ch)
	require.Error(t, err)
	require.Nil(t, data)
	require.Equal(t, testError.Error(), err.Error())
}

func TestReceive_ClosedChannel(t *testing.T) {
	ch := make(Chan[string])
	Close[string](ch)

	data, err := Receive[string](ch)
	require.Error(t, err)
	require.Nil(t, data)
	require.Equal(t, ErrReceiveClosed, err)
}

func TestCollect(t *testing.T) {
	ch := make(Chan[string], 3)

	Send(ch, stringPtr("item1"), nil)
	Send(ch, stringPtr("item2"), nil)
	Send(ch, stringPtr("item3"), nil)
	Close[string](ch)

	results, err := Collect[string](ch)
	require.NoError(t, err)
	require.Len(t, results, 3)
	require.Equal(t, "item1", *results[0])
	require.Equal(t, "item2", *results[1])
	require.Equal(t, "item3", *results[2])
}

func TestCollect_WithError(t *testing.T) {
	ch := make(Chan[string], 2)
	testError := errors.New("test error")

	Send(ch, stringPtr("item1"), nil)
	Send[string](ch, nil, testError)

	results, err := Collect[string](ch)
	require.Error(t, err)
	require.Nil(t, results)
	require.Equal(t, testError.Error(), err.Error())
}

func TestCollect_EmptyChannel(t *testing.T) {
	ch := make(Chan[string])
	Close[string](ch)

	results, err := Collect[string](ch)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestCollect_MixedData(t *testing.T) {
	ch := make(Chan[string], 4)

	Send(ch, stringPtr("item1"), nil)
	Send(ch, stringPtr("item2"), nil)
	Send[string](ch, nil, errors.New("error1"))
	Send(ch, stringPtr("item3"), nil)

	results, err := Collect[string](ch)
	require.Error(t, err)
	require.Nil(t, results)
	require.Equal(t, "error1", err.Error())
}

func TestChanResult_ComplexType(t *testing.T) {
	type TestStruct struct {
		ID   int
		Name string
	}

	ch := make(Chan[TestStruct], 1)
	testData := &TestStruct{ID: 1, Name: "test"}

	Send(ch, testData, nil)

	result := <-ch
	require.NotNil(t, result)
	require.NoError(t, result.Error)
	require.Equal(t, testData, result.data)

	data, err := result.Get()
	require.NoError(t, err)
	require.Equal(t, testData, data)
}

func TestChanResult_NilData(t *testing.T) {
	ch := make(Chan[string], 1)

	Send[string](ch, nil, nil)

	result := <-ch
	require.NotNil(t, result)
	require.NoError(t, result.Error)
	require.Nil(t, result.data)

	data, err := result.Get()
	require.NoError(t, err)
	require.Nil(t, data)
}

func stringPtr(s string) *string {
	return &s
}
