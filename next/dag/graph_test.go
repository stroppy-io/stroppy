package dag

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBuildValidation(t *testing.T) {
	t.Run("empty id", func(t *testing.T) {
		_, err := NewGraph().Add(&Node{ID: "", Run: okRun}).Build()
		if !errors.Is(err, ErrEmptyID) {
			t.Fatalf("err = %v, want ErrEmptyID", err)
		}
	})

	t.Run("duplicate id", func(t *testing.T) {
		_, err := NewGraph().Add(okNode("a")).Add(okNode("a")).Build()
		if !errors.Is(err, ErrDuplicateID) {
			t.Fatalf("err = %v, want ErrDuplicateID", err)
		}
	})

	t.Run("unknown dep", func(t *testing.T) {
		_, err := NewGraph().Add(afterNode("a", "ghost")).Build()
		if !errors.Is(err, ErrUnknownDep) {
			t.Fatalf("err = %v, want ErrUnknownDep", err)
		}
	})

	t.Run("cycle reports the path", func(t *testing.T) {
		_, err := NewGraph().
			Add(afterNode("a", "b")).
			Add(afterNode("b", "c")).
			Add(afterNode("c", "a")).
			Build()
		if !errors.Is(err, ErrCycle) {
			t.Fatalf("err = %v, want ErrCycle", err)
		}

		for _, id := range []string{"a", "b", "c"} {
			if !strings.Contains(err.Error(), id) {
				t.Fatalf("cycle error %q does not mention %q", err, id)
			}
		}
	})

	t.Run("self-loop is a cycle", func(t *testing.T) {
		_, err := NewGraph().Add(afterNode("a", "a")).Build()
		if !errors.Is(err, ErrCycle) {
			t.Fatalf("err = %v, want ErrCycle", err)
		}
	})

	t.Run("empty graph is valid and runs to Succeeded", func(t *testing.T) {
		b, err := NewGraph().Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		res := Run(context.Background(), b)
		if res.Status != Succeeded {
			t.Fatalf("run status = %v, want Succeeded", res.Status)
		}

		if len(res.Nodes) != 0 {
			t.Fatalf("expected no node results, got %d", len(res.Nodes))
		}
	})
}
