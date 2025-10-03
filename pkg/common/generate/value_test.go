package generate

import (
	"testing"

	pb "github.com/stroppy-io/stroppy/pkg/common/proto"
	"google.golang.org/protobuf/proto"
)

func ptr[T any](x T) *T {
	return &x
}

func TestNewTupleGenerator(t *testing.T) {
	type args struct {
		seed     uint64
		genInfos []GenAbleStruct
	}
	tests := []struct {
		name string
		args args
		want [][]*pb.Value
	}{
		{name: "simple",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "w_id",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 1, Max: 2},
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "d_id",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 10, Max: 12},
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]*pb.Value{
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 10}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 11}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 12}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 10}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 11}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 12}},
				},
			},
		},
		{
			name: "empty_genInfos",
			args: args{seed: 1, genInfos: []GenAbleStruct{}},
			want: [][]*pb.Value{},
		},
		{
			name: "single_parameter",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "id",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 5, Max: 7},
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]*pb.Value{
				[]*pb.Value{&pb.Value{Type: &pb.Value_Int64{Int64: 5}}},
				[]*pb.Value{&pb.Value{Type: &pb.Value_Int64{Int64: 6}}},
				[]*pb.Value{&pb.Value{Type: &pb.Value_Int64{Int64: 7}}},
			},
		},
		{
			name: "single_value_range_min_equals_max",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "fixed_id",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 42, Max: 42},
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "range_id",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 1, Max: 2},
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]*pb.Value{
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 42}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 42}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 2}},
				},
			},
		},
		{
			name: "three_parameters",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "a",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 1, Max: 2},
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "b",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 10, Max: 11},
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "c",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 100, Max: 101},
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]*pb.Value{
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 10}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 100}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 10}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 101}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 11}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 100}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 11}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 101}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 10}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 100}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 10}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 101}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 11}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 100}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 11}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 101}},
				},
			},
		},
		{
			name: "zero_and_negative_boundaries",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "negative",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: -2, Max: 0},
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "crossing_zero",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: -1, Max: 1},
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]*pb.Value{
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: -2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: -1}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: -2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 0}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: -2}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: -1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: -1}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: -1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 0}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: -1}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 0}},
					&pb.Value{Type: &pb.Value_Int64{Int64: -1}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 0}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 0}},
				},
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 0}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 1}},
				},
			},
		},
		{
			name: "both_params_single_value",
			args: args{seed: 1, genInfos: []GenAbleStruct{
				&pb.QueryParamDescriptor{
					Name: "fixed_a",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 7, Max: 7},
							},
						},
						Unique: ptr(true),
					},
				},
				&pb.QueryParamDescriptor{
					Name: "fixed_b",
					GenerationRule: &pb.Generation_Rule{
						Type: &pb.Generation_Rule_Int64Rules{
							Int64Rules: &pb.Generation_Rules_Int64Rule{
								Range: &pb.Generation_Range_Int64Range{Min: 9, Max: 9},
							},
						},
						Unique: ptr(true),
					},
				},
			}},
			want: [][]*pb.Value{
				[]*pb.Value{
					&pb.Value{Type: &pb.Value_Int64{Int64: 7}},
					&pb.Value{Type: &pb.Value_Int64{Int64: 9}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewTupleGenerator(tt.args.seed, tt.args.genInfos...)
			for i, pair := range tt.want {
				wrapped := &pb.Value{Type: &pb.Value_List_{List: &pb.Value_List{Values: pair}}}
				got, err := gen.Next()
				if err != nil {
					t.Errorf("generator returned error: %s", err)
				}
				if !proto.Equal(
					got,
					wrapped,
				) {
					t.Errorf("i=%d) NewTupleGenerator().Next() = %v, want %v", i, got, wrapped)
				}
			}
		})
	}
}
