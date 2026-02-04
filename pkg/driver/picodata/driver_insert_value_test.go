package picodata

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func ptr[T any](v T) *T {
	val := v
	return &val
}

func TestDriver_InsertValuesPlainQuery(t *testing.T) {
	t.Run("single row insert successfully", func(t *testing.T) {
		mock := &mockPool{}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		descriptor := &stroppy.InsertDescriptor{
			Name:      "test_insert_single",
			TableName: "test_table",
			Method:    stroppy.InsertMethod_PLAIN_QUERY.Enum(),
			Params: []*stroppy.QueryParamDescriptor{
				{
					Name: "id",
					GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_Int64Range{
							Int64Range: &stroppy.Generation_Range_Int64{
								Min: ptr[int64](1),
								Max: 100,
							},
						},
						Unique: ptr(true),
					},
				},
			},
		}

		count := int64(1)

		stats, err := drv.InsertValues(context.Background(), descriptor, count)
		require.NoError(t, err)
		require.NotNil(t, stats)
		require.Len(t, stats.Queries, 1)
		require.Equal(t, "test_insert_single", stats.Queries[0].Name)

		require.Len(t, mock.execCalls, 1)
		require.Contains(t, mock.execCalls[0].SQL, "insert into test_table")
	})

	t.Run("multiple rows insert successfully", func(t *testing.T) {
		mock := &mockPool{}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		descriptor := &stroppy.InsertDescriptor{
			Name:      "test_insert_multi",
			TableName: "test_table",
			Method:    stroppy.InsertMethod_PLAIN_QUERY.Enum(),
			Params: []*stroppy.QueryParamDescriptor{
				{
					Name: "id",
					GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_Int64Range{
							Int64Range: &stroppy.Generation_Range_Int64{
								Min: ptr[int64](1),
								Max: 1000,
							},
						},
						Unique: ptr(true),
					},
				},
				{
					Name: "name",
					GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_StringConst{
							StringConst: "test_user",
						},
					},
				},
			},
		}

		count := int64(5)

		stats, err := drv.InsertValues(context.Background(), descriptor, count)
		require.NoError(t, err)
		require.NotNil(t, stats)
		require.Len(t, stats.Queries, 1)
		require.Equal(t, "test_insert_multi", stats.Queries[0].Name)

		require.Len(t, mock.execCalls, 5)
		for _, call := range mock.execCalls {
			require.Contains(t, call.SQL, "insert into test_table")
		}
	})

	t.Run("zero count handled", func(t *testing.T) {
		mock := &mockPool{}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		descriptor := &stroppy.InsertDescriptor{
			Name:      "test_insert_zero",
			TableName: "test_table",
			Method:    stroppy.InsertMethod_PLAIN_QUERY.Enum(),
			Params: []*stroppy.QueryParamDescriptor{
				{
					Name: "id",
					GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_Int64Range{
							Int64Range: &stroppy.Generation_Range_Int64{
								Min: ptr[int64](1),
								Max: 100,
							},
						},
					},
				},
			},
		}

		count := int64(0)

		stats, err := drv.InsertValues(context.Background(), descriptor, count)
		require.NoError(t, err)
		require.NotNil(t, stats)

		require.Len(t, mock.execCalls, 0)
	})

	t.Run("on exec failure returns error", func(t *testing.T) {
		mock := &mockPool{
			execErr: errors.New("test_error"),
		}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		descriptor := &stroppy.InsertDescriptor{
			Name:      "test_insert_fail",
			TableName: "test_table",
			Method:    stroppy.InsertMethod_PLAIN_QUERY.Enum(),
			Params: []*stroppy.QueryParamDescriptor{
				{
					Name: "id",
					GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_Int64Range{
							Int64Range: &stroppy.Generation_Range_Int64{
								Min: ptr[int64](1),
								Max: 100,
							},
						},
					},
				},
			},
		}

		count := int64(1)

		stats, err := drv.InsertValues(context.Background(), descriptor, count)
		require.Error(t, err)
		require.Nil(t, stats)
		require.Contains(t, err.Error(), "test_error")
	})
}

func TestDriver_InsertValues(t *testing.T) {
	t.Run("PlainQuery successfully works", func(t *testing.T) {
		mock := &mockPool{}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		descriptor := &stroppy.InsertDescriptor{
			Name:      "test_insert",
			TableName: "test_table",
			Method:    stroppy.InsertMethod_PLAIN_QUERY.Enum(),
			Params: []*stroppy.QueryParamDescriptor{
				{
					Name: "id",
					GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_Int64Range{
							Int64Range: &stroppy.Generation_Range_Int64{
								Min: ptr[int64](1),
								Max: 100,
							},
						},
						Unique: ptr(true),
					},
				},
			},
		}

		count := int64(2)

		stats, err := drv.InsertValues(context.Background(), descriptor, count)
		require.NoError(t, err)
		require.NotNil(t, stats)
		require.Len(t, mock.execCalls, 2)
	})

	t.Run("CopyFrom fails", func(t *testing.T) {
		mock := &mockPool{}
		drv, err := newMockPool(mock)
		require.NoError(t, err)

		descriptor := &stroppy.InsertDescriptor{
			Name:      "test_copy",
			TableName: "test_table",
			Method:    stroppy.InsertMethod_COPY_FROM.Enum(),
			Params: []*stroppy.QueryParamDescriptor{
				{
					Name: "id",
					GenerationRule: &stroppy.Generation_Rule{
						Kind: &stroppy.Generation_Rule_Int64Range{
							Int64Range: &stroppy.Generation_Range_Int64{
								Min: ptr[int64](1),
								Max: 100,
							},
						},
					},
				},
			},
		}

		count := int64(10)

		stats, err := drv.InsertValues(context.Background(), descriptor, count)
		require.Error(t, err)
		require.Nil(t, stats)
		require.Equal(t, err, ErrCopyFromUnsupported)
	})
}
