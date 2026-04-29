package expr

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stroppy-io/stroppy/pkg/datagen/dgproto"
)

func TestLiteralArms(t *testing.T) {
	ts := time.Date(2020, 6, 15, 12, 30, 0, 0, time.UTC)

	cases := []struct {
		name  string
		lit   *dgproto.Literal
		check func(t *testing.T, v any)
	}{
		{
			name: "int64",
			lit:  &dgproto.Literal{Value: &dgproto.Literal_Int64{Int64: -42}},
			check: func(t *testing.T, v any) {
				t.Helper()

				if v != int64(-42) {
					t.Fatalf("got %v", v)
				}
			},
		},
		{
			name: "double",
			lit:  &dgproto.Literal{Value: &dgproto.Literal_Double{Double: 3.14}},
			check: func(t *testing.T, v any) {
				t.Helper()

				if v != 3.14 {
					t.Fatalf("got %v", v)
				}
			},
		},
		{
			name: "string",
			lit:  &dgproto.Literal{Value: &dgproto.Literal_String_{String_: "hi"}},
			check: func(t *testing.T, v any) {
				t.Helper()

				if v != "hi" {
					t.Fatalf("got %v", v)
				}
			},
		},
		{
			name: "bool",
			lit:  &dgproto.Literal{Value: &dgproto.Literal_Bool{Bool: true}},
			check: func(t *testing.T, v any) {
				t.Helper()

				if v != true {
					t.Fatalf("got %v", v)
				}
			},
		},
		{
			name: "bytes",
			lit:  &dgproto.Literal{Value: &dgproto.Literal_Bytes{Bytes: []byte{0x01, 0x02}}},
			check: func(t *testing.T, v any) {
				t.Helper()

				b, ok := v.([]byte)
				if !ok || !bytes.Equal(b, []byte{0x01, 0x02}) {
					t.Fatalf("got %v", v)
				}
			},
		},
		{
			name: "timestamp",
			lit:  &dgproto.Literal{Value: &dgproto.Literal_Timestamp{Timestamp: timestamppb.New(ts)}},
			check: func(t *testing.T, v any) {
				t.Helper()

				got, ok := v.(time.Time)
				if !ok || !got.Equal(ts) {
					t.Fatalf("got %v", v)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: tc.lit}}

			got, err := Eval(newFakeCtx(), e)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			tc.check(t, got)
		})
	}
}

func TestLiteralEmpty(t *testing.T) {
	e := &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{}}}
	if _, err := Eval(newFakeCtx(), e); !errors.Is(err, ErrBadExpr) {
		t.Fatalf("want ErrBadExpr, got %v", err)
	}
}

// TestLiteralNull verifies the Null arm evaluates to (nil, nil). The nil
// return is the row-scratch SQL NULL representation for drivers.
func TestLiteralNull(t *testing.T) {
	e := &dgproto.Expr{Kind: &dgproto.Expr_Lit{Lit: &dgproto.Literal{
		Value: &dgproto.Literal_Null{Null: &dgproto.NullMarker{}},
	}}}

	got, err := Eval(newFakeCtx(), e)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if got != nil {
		t.Fatalf("got %v (%T), want nil", got, got)
	}
}
