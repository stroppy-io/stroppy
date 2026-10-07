// Package testdriver scopes database-free backend selection to one test run.
package testdriver

import (
	"context"

	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

// Binding selects a repository-provided test backend, not an arbitrary driver.
type Binding struct {
	Kind     string
	Recorder *record.Recorder
}

type key struct{}

func WithContext(ctx context.Context, binding Binding) context.Context {
	return context.WithValue(ctx, key{}, binding)
}

func FromContext(ctx context.Context) Binding {
	if ctx == nil {
		return Binding{}
	}

	binding, _ := ctx.Value(key{}).(Binding)

	return binding
}
