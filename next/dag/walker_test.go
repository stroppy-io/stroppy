package dag

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

var okRun = func(context.Context) error { return nil }

func okNode(id string) *Node {
	return &Node{ID: id, Run: okRun}
}

func afterNode(id string, deps ...string) *Node {
	return &Node{ID: id, Run: okRun, After: deps}
}

func failRun(msg string) func(context.Context) error {
	return func(context.Context) error { return errors.New(msg) }
}

func waitForCtx() func(context.Context) error {
	return func(ctx context.Context) error {
		<-ctx.Done()

		return ctx.Err()
	}
}

func requireStatus(t *testing.T, res *RunResult, id string, want Status) {
	t.Helper()

	n := res.Node(id)
	if n == nil {
		t.Fatalf("node %q missing from result", id)
	}

	if n.Status != want {
		t.Fatalf("node %q status = %v, want %v (err=%v)", id, n.Status, want, n.Err)
	}
}

// assertOrder checks that before's Run visibly completed no later than
// after's Run started, evidencing the walker gated after on before.
func assertOrder(t *testing.T, res *RunResult, before, after string) {
	t.Helper()

	b, a := res.Node(before), res.Node(after)
	if b == nil || a == nil {
		t.Fatalf("missing nodes for order check %q -> %q", before, after)
	}

	if b.End.After(a.Start) {
		t.Fatalf("%q (end %v) did not precede %q (start %v)", before, b.End, after, a.Start)
	}
}

// TestShapes covers linear chain, diamond, fan-out and independent
// component topologies, asserting both terminal status and the
// dependency ordering the walker is meant to enforce.
func TestShapes(t *testing.T) {
	tests := []struct {
		name  string
		build func() *Graph
		check func(t *testing.T, res *RunResult)
	}{
		{
			name: "linear chain",
			build: func() *Graph {
				return NewGraph().
					Add(okNode("a")).
					Add(afterNode("b", "a")).
					Add(afterNode("c", "b")).
					Add(afterNode("d", "c"))
			},
			check: func(t *testing.T, res *RunResult) {
				t.Helper()

				for _, id := range []string{"a", "b", "c", "d"} {
					requireStatus(t, res, id, Succeeded)
				}

				assertOrder(t, res, "a", "b")
				assertOrder(t, res, "b", "c")
				assertOrder(t, res, "c", "d")
			},
		},
		{
			name: "diamond",
			build: func() *Graph {
				return NewGraph().
					Add(okNode("a")).
					Add(afterNode("b", "a")).
					Add(afterNode("c", "a")).
					Add(afterNode("d", "b", "c"))
			},
			check: func(t *testing.T, res *RunResult) {
				t.Helper()

				for _, id := range []string{"a", "b", "c", "d"} {
					requireStatus(t, res, id, Succeeded)
				}

				assertOrder(t, res, "a", "b")
				assertOrder(t, res, "a", "c")
				assertOrder(t, res, "b", "d")
				assertOrder(t, res, "c", "d")
			},
		},
		{
			name: "fan-out(10)",
			build: func() *Graph {
				g := NewGraph().Add(okNode("a"))
				for i := range 10 {
					g.Add(afterNode(fmt.Sprintf("b%d", i), "a"))
				}

				return g
			},
			check: func(t *testing.T, res *RunResult) {
				t.Helper()

				requireStatus(t, res, "a", Succeeded)

				for i := range 10 {
					id := fmt.Sprintf("b%d", i)
					requireStatus(t, res, id, Succeeded)
					assertOrder(t, res, "a", id)
				}
			},
		},
		{
			name: "independent components",
			build: func() *Graph {
				return NewGraph().
					Add(okNode("x1")).Add(afterNode("x2", "x1")).
					Add(okNode("y1")).Add(afterNode("y2", "y1"))
			},
			check: func(t *testing.T, res *RunResult) {
				t.Helper()

				for _, id := range []string{"x1", "x2", "y1", "y2"} {
					requireStatus(t, res, id, Succeeded)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := tc.build().Build()
			if err != nil {
				t.Fatalf("build: %v", err)
			}

			res := Run(context.Background(), b)
			if res.Status != Succeeded {
				t.Fatalf("run status = %v, want Succeeded", res.Status)
			}

			tc.check(t, res)
		})
	}
}

// TestFailurePolicies covers the three FailurePolicy values and the
// OnFailure cleanup edge.
func TestFailurePolicies(t *testing.T) {
	t.Run("AbortRun cancels in-flight and pending", func(t *testing.T) {
		g := NewGraph().
			Add(&Node{ID: "a", Run: failRun("boom"), Failure: AbortRun}).
			Add(afterNode("b", "a")).
			Add(&Node{ID: "c", Run: waitForCtx()})

		b, err := g.Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		res := Run(context.Background(), b)
		requireStatus(t, res, "a", Failed)
		requireStatus(t, res, "b", Canceled)
		requireStatus(t, res, "c", Canceled)

		if res.Status != Failed {
			t.Fatalf("run status = %v, want Failed", res.Status)
		}
	})

	t.Run("SkipDependents skips exactly the success-descendant closure", func(t *testing.T) {
		g := NewGraph().
			Add(&Node{ID: "a", Run: failRun("boom"), Failure: SkipDependents}).
			Add(afterNode("b", "a")).
			Add(afterNode("c", "b")).
			Add(okNode("d"))

		b, err := g.Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		res := Run(context.Background(), b)
		requireStatus(t, res, "a", Failed)
		requireStatus(t, res, "b", Skipped)
		requireStatus(t, res, "c", Skipped)
		requireStatus(t, res, "d", Succeeded)

		if res.Status != Failed {
			t.Fatalf("run status = %v, want Failed", res.Status)
		}
	})

	t.Run("Continue lets dependents proceed", func(t *testing.T) {
		g := NewGraph().
			Add(&Node{ID: "a", Run: failRun("boom"), Failure: Continue}).
			Add(afterNode("b", "a"))

		b, err := g.Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		res := Run(context.Background(), b)
		requireStatus(t, res, "a", Failed)
		requireStatus(t, res, "b", Succeeded)

		if res.Status != Failed {
			t.Fatalf("run status = %v, want Failed", res.Status)
		}
	})

	t.Run("OnFailure node runs only on failure", func(t *testing.T) {
		g := NewGraph().
			Add(&Node{ID: "a", Run: failRun("boom"), Failure: SkipDependents}).
			Add(&Node{ID: "cleanup", Run: okRun, OnFailure: []string{"a"}})

		b, err := g.Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		res := Run(context.Background(), b)
		requireStatus(t, res, "a", Failed)
		requireStatus(t, res, "cleanup", Succeeded)
	})

	t.Run("OnFailure node is Skipped on success", func(t *testing.T) {
		g := NewGraph().
			Add(okNode("a")).
			Add(&Node{ID: "cleanup", Run: okRun, OnFailure: []string{"a"}})

		b, err := g.Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		res := Run(context.Background(), b)
		requireStatus(t, res, "a", Succeeded)
		requireStatus(t, res, "cleanup", Skipped)
	})

	// OnFailure-gated nodes are exempt from AbortRun's cancellation:
	// with AbortRun the default policy, cleanup must still run after an
	// abort. Ordinary dependents are canceled as usual.
	t.Run("OnFailure cleanup runs under AbortRun", func(t *testing.T) {
		g := NewGraph().
			Add(&Node{ID: "a", Run: failRun("boom"), Failure: AbortRun}).
			Add(afterNode("b", "a")).
			Add(&Node{ID: "cleanup", Run: okRun, OnFailure: []string{"a"}})

		b, err := g.Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		res := Run(context.Background(), b)
		requireStatus(t, res, "a", Failed)
		requireStatus(t, res, "b", Canceled)
		requireStatus(t, res, "cleanup", Succeeded)
	})

	// The exemption does not shield cleanup from the caller's context:
	// external cancellation (e.g. Ctrl-C) cancels OnFailure nodes too.
	t.Run("OnFailure cleanup is canceled by external ctx cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		started := make(chan struct{})

		g := NewGraph().
			Add(&Node{ID: "a", Run: failRun("boom"), Failure: AbortRun}).
			Add(&Node{ID: "cleanup", OnFailure: []string{"a"}, Run: func(ctx context.Context) error {
				close(started)
				<-ctx.Done()

				return ctx.Err()
			}})

		b, err := g.Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		go func() {
			<-started
			cancel()
		}()

		start := time.Now()
		res := Run(ctx, b)

		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("Run took %v after external cancel, want prompt return", elapsed)
		}

		requireStatus(t, res, "a", Failed)
		requireStatus(t, res, "cleanup", Canceled)
	})

	// Mixed gating: an OnFailure node's plain After deps still evaluate
	// normally against terminal statuses; the exemption only concerns
	// which context the node runs against.
	t.Run("OnFailure with After on succeeded node runs under AbortRun", func(t *testing.T) {
		g := NewGraph().
			Add(okNode("u")).
			Add(&Node{ID: "a", Run: failRun("boom"), After: []string{"u"}, Failure: AbortRun}).
			Add(&Node{ID: "cleanup", Run: okRun, OnFailure: []string{"a"}, After: []string{"u"}})

		b, err := g.Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}

		res := Run(context.Background(), b)
		requireStatus(t, res, "u", Succeeded)
		requireStatus(t, res, "a", Failed)
		requireStatus(t, res, "cleanup", Succeeded)
	})
}

// TestIfPredicatePrune checks that a false If skips the node and its
// success-dependent closure, while unrelated nodes are unaffected.
func TestIfPredicatePrune(t *testing.T) {
	g := NewGraph().
		Add(&Node{ID: "a", Run: okRun, If: func() bool { return false }}).
		Add(afterNode("b", "a")).
		Add(okNode("c"))

	b, err := g.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	res := Run(context.Background(), b)
	requireStatus(t, res, "a", Skipped)
	requireStatus(t, res, "b", Skipped)
	requireStatus(t, res, "c", Succeeded)

	if res.Status != Succeeded {
		t.Fatalf("run status = %v, want Succeeded (Skipped is not a failure)", res.Status)
	}
}

// TestAfterAny checks that a node gated on AfterAny runs once one listed
// dependency succeeds even though another failed.
func TestAfterAny(t *testing.T) {
	g := NewGraph().
		Add(&Node{ID: "a", Run: failRun("boom"), Failure: SkipDependents}).
		Add(okNode("b")).
		Add(&Node{ID: "d", Run: okRun, AfterAny: []string{"a", "b"}})

	b, err := g.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	res := Run(context.Background(), b)
	requireStatus(t, res, "a", Failed)
	requireStatus(t, res, "b", Succeeded)
	requireStatus(t, res, "d", Succeeded)
}

// TestExternalCancelMidRun checks that canceling the caller's context
// resolves in-flight nodes to Canceled and that Run returns promptly
// rather than waiting out the node's own timeout.
func TestExternalCancelMidRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	n := &Node{
		ID: "slow",
		Run: func(ctx context.Context) error {
			select {
			case <-time.After(10 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	b, err := NewGraph().Add(n).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	res := Run(ctx, b)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("Run took %v after external cancel, want promptly under the node's 10s sleep", elapsed)
	}

	requireStatus(t, res, "slow", Canceled)

	if res.Status != Canceled {
		t.Fatalf("run status = %v, want Canceled", res.Status)
	}
}

// TestPanicInNode checks that a panic is recovered as a Failed status
// carrying the panic value, and that the run continues per the node's
// failure policy.
func TestPanicInNode(t *testing.T) {
	g := NewGraph().
		Add(&Node{
			ID:      "boom",
			Run:     func(context.Context) error { panic("kaboom") },
			Failure: SkipDependents,
		}).
		Add(okNode("sibling"))

	b, err := g.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	res := Run(context.Background(), b)
	requireStatus(t, res, "boom", Failed)

	if res.Node("boom").Err == nil || !strings.Contains(res.Node("boom").Err.Error(), "kaboom") {
		t.Fatalf("panic message not captured: %v", res.Node("boom").Err)
	}

	requireStatus(t, res, "sibling", Succeeded)
}

// TestNoGoroutineLeak runs a moderately wide graph and checks the
// goroutine count settles back to its pre-run baseline, since Run must
// return only after every node goroutine it launched has exited.
func TestNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()

	g := NewGraph().Add(okNode("root"))
	for i := range 50 {
		g.Add(afterNode(fmt.Sprintf("n%d", i), "root"))
	}

	b, err := g.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	Run(context.Background(), b)

	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		runtime.Gosched()
	}

	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutine leak: before=%d after=%d", before, after)
	}
}
