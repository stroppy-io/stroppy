package bench

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
)

func TestRunCatalogRejectsFactoryNameChange(t *testing.T) {
	calls := 0
	factory := func() Workload {
		calls++

		name := "test/catalog-name"
		if calls > 1 {
			name = "test/different-name"
		}

		return &catalogTestWorkload{name: name}
	}

	catalog, err := NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}

	err = RunCatalog(
		t.Context(),
		catalog,
		"test/catalog-name",
		map[int]*config.DriverConfig{0: {DriverType: config.DriverTypeNoop}},
		ParamInputs{},
		nil,
		nil,
		zap.NewNop(),
		&MetricsConfig{Quiet: true},
	)
	if err == nil || !strings.Contains(err.Error(), "different name") {
		t.Fatalf("RunCatalog() error = %v", err)
	}
}

type catalogTestWorkload struct{ name string }

func (workload *catalogTestWorkload) Name() string                  { return workload.name }
func (*catalogTestWorkload) Define(*Def) error                      { return nil }
func (*catalogTestWorkload) Setup(context.Context, *Bench) error    { return nil }
func (*catalogTestWorkload) Iterate(context.Context, *Bench) error  { return nil }
func (*catalogTestWorkload) Teardown(context.Context, *Bench) error { return nil }
