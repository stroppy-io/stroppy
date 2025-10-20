package queries

import (
	"testing"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

func Test_badInsertSQL(t *testing.T) {
	type args struct {
		descriptor *stroppy.InsertDescriptor
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "COPY FROM",
			args: args{
				descriptor: &stroppy.InsertDescriptor{
					Name:      "simple_name",
					TableName: "simple_table",
					Method:    stroppy.InsertMethod_COPY_FROM.Enum(),
					Params:    []*stroppy.QueryParamDescriptor{{Name: "a"}, {Name: "b"}},
					Groups: []*stroppy.QueryParamGroup{
						{Params: []*stroppy.QueryParamDescriptor{{Name: "c"}, {Name: "d"}}},
					},
				},
			},
			want: "simple_table a b c d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := badInsertSQL(tt.args.descriptor); got != tt.want {
				t.Errorf("badInsertSQL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_insertSQL(t *testing.T) {
	type args struct {
		descriptor *stroppy.InsertDescriptor
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "plain queries",
			args: args{
				descriptor: &stroppy.InsertDescriptor{
					Name:      "simple_name",
					TableName: "simple_table",
					Method:    stroppy.InsertMethod_PLAIN_QUERY.Enum(),
					Params:    []*stroppy.QueryParamDescriptor{{Name: "a"}, {Name: "b"}},
					Groups: []*stroppy.QueryParamGroup{
						{Params: []*stroppy.QueryParamDescriptor{{Name: "c"}, {Name: "d"}}},
					},
				},
			},
			want: "insert into simple_table (a, b, c, d) values ($1, $2, $3, $4)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := insertSQL(tt.args.descriptor); got != tt.want {
				t.Errorf("insertSQL() = %v, want %v", got, tt.want)
			}
		})
	}
}
