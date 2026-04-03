package generate

import (
	"testing"

	pb "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func ptr[T any](x T) *T {
	return &x
}

//nolint:maintidx // table tests supposed to be long
func TestNewTupleGenerator(t *testing.T) {
	type args struct {
		seed     uint64
		genInfos []GenAbleStruct
	}

	tests := []struct {
		name string
		args args
		want [][]any
	}{
		{
			name: "simple",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "w_id",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](1), Max: 2,
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "d_id",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](10),
								Max: 12,
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]any{
				{int64(1), int64(10)},
				{int64(1), int64(11)},
				{int64(1), int64(12)},
				{int64(2), int64(10)},
				{int64(2), int64(11)},
				{int64(2), int64(12)},
			},
		},
		{
			name: "empty_genInfos",
			args: args{seed: 1, genInfos: []GenAbleStruct{}},
			want: [][]any{},
		},
		{
			name: "single_parameter",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "id",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](5), Max: 7,
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]any{
				{int64(5)},
				{int64(6)},
				{int64(7)},
			},
		},
		{
			name: "single_value_range_min_equals_max",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "fixed_id",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](42),
								Max: 42,
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "range_id",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](1), Max: 2,
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]any{
				{int64(42), int64(1)},
				{int64(42), int64(2)},
			},
		},
		{
			name: "three_parameters",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "a",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](1), Max: 2,
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "b",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](10), Max: 11,
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "c",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](100),
								Max: 101,
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]any{
				{int64(1), int64(10), int64(100)},
				{int64(1), int64(10), int64(101)},
				{int64(1), int64(11), int64(100)},
				{int64(1), int64(11), int64(101)},
				{int64(2), int64(10), int64(100)},
				{int64(2), int64(10), int64(101)},
				{int64(2), int64(11), int64(100)},
				{int64(2), int64(11), int64(101)},
			},
		},
		{
			name: "zero_and_negative_boundaries",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "negative",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](-2), Max: 0,
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "crossing_zero",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](-1), Max: 1,
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]any{
				{int64(-2), int64(-1)},
				{int64(-2), int64(0)},
				{int64(-2), int64(1)},
				{int64(-1), int64(-1)},
				{int64(-1), int64(0)},
				{int64(-1), int64(1)},
				{int64(0), int64(-1)},
				{int64(0), int64(0)},
				{int64(0), int64(1)},
			},
		},
		{
			name: "both_params_single_value",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "fixed_a",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](7), Max: 7,
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "fixed_b",
					GenerationRule: &pb.Generation_Rule{
						Kind: &pb.Generation_Rule_Int64Range{
							Int64Range: &pb.Generation_Range_Int64{
								Min: ptr[int64](9), Max: 9,
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]any{
				{int64(7), int64(9)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewTupleGenerator(tt.args.seed, tt.args.genInfos)

			for i, pair := range tt.want {
				got, err := gen.Next()
				if err != nil {
					t.Errorf("generator returned error: %s", err)
				}

				gotSlice, ok := got.([]any)
				if !ok {
					t.Errorf("i=%d) expected []any, got %T", i, got)

					continue
				}

				if len(gotSlice) != len(pair) {
					t.Errorf("i=%d) len mismatch: got %d, want %d", i, len(gotSlice), len(pair))

					continue
				}

				for j, exp := range pair {
					if gotSlice[j] != exp {
						t.Errorf("i=%d j=%d) got %v (%T), want %v (%T)", i, j, gotSlice[j], gotSlice[j], exp, exp)
					}
				}
			}
		})
	}
}
