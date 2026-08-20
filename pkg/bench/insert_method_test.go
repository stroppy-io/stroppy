package bench

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

func TestResolveInsertMethodPrecedence(t *testing.T) {
	explicit := func(method string) Param[string] {
		return Param[string]{value: method, source: ParamSourceCLI}
	}
	unset := func() Param[string] { return Param[string]{source: ParamSourceDefault} }

	t.Run("typed override wins over driver default", func(t *testing.T) {
		got, err := resolveInsertMethod(explicit("columnar"), &config.DriverConfig{
			DriverType:   config.DriverTypePostgres,
			InsertMethod: "native",
		})
		if err != nil {
			t.Fatalf("resolveInsertMethod(): %v", err)
		}

		if got != driver.InsertColumnar {
			t.Fatalf("resolveInsertMethod() = %v, want columnar", got)
		}
	})

	t.Run("driver default used when typed param unset", func(t *testing.T) {
		got, err := resolveInsertMethod(unset(), &config.DriverConfig{
			DriverType:   config.DriverTypePostgres,
			InsertMethod: "native",
		})
		if err != nil {
			t.Fatalf("resolveInsertMethod(): %v", err)
		}

		if got != driver.InsertNative {
			t.Fatalf("resolveInsertMethod() = %v, want native", got)
		}
	})

	t.Run("workload default when nothing set", func(t *testing.T) {
		got, err := resolveInsertMethod(unset(), &config.DriverConfig{
			DriverType: config.DriverTypePostgres,
		})
		if err != nil {
			t.Fatalf("resolveInsertMethod(): %v", err)
		}

		if got != 0 {
			t.Fatalf("resolveInsertMethod() = %v, want zero (workload default)", got)
		}
	})

	t.Run("invalid typed value rejected", func(t *testing.T) {
		_, err := resolveInsertMethod(explicit("bogus"), &config.DriverConfig{
			DriverType: config.DriverTypePostgres,
		})
		if !errors.Is(err, driver.ErrUnknownInsertMethod) {
			t.Fatalf("resolveInsertMethod() error = %v, want ErrUnknownInsertMethod", err)
		}
	})

	t.Run("unsupported typed value rejected", func(t *testing.T) {
		_, err := resolveInsertMethod(explicit("columnar"), &config.DriverConfig{
			DriverType: config.DriverTypeMySQL,
		})
		if err == nil {
			t.Fatal("resolveInsertMethod() accepted columnar for mysql")
		}
	})
}

func validInsertSource(total int64) *gen.IndexedSource {
	b := gen.NewSchemaBuilder()
	id := b.Int64("id")

	return gen.NewIndexedSource(b.Build(), gen.Root{}, "test/insert-method@1", total, 1,
		func(r gen.Row, entity uint64) error {
			r.SetInt64(id, int64(entity))

			return nil
		})
}

// recordingDriver captures the method of the last InsertRequest it served.
type recordingDriver struct {
	method driver.InsertMethod
}

func (d *recordingDriver) Insert(_ context.Context, req *driver.InsertRequest) (*stats.Query, error) {
	d.method = req.Method

	return &stats.Query{Rows: 1}, nil
}

func (d *recordingDriver) RunQuery(context.Context, string, map[string]any) (*driver.QueryResult, error) {
	return nil, errors.New("unexpected RunQuery")
}

func (d *recordingDriver) Begin(context.Context, config.TxIsolationLevel) (driver.Tx, error) {
	return nil, errors.New("unexpected Begin")
}

func (d *recordingDriver) ClassifyError(error) driver.ErrorFacts { return driver.ErrorFacts{} }

func (d *recordingDriver) Teardown(context.Context) error { return nil }

func TestInsertAppliesResolvedMethod(t *testing.T) {
	installRuntimeTestRoot(t)

	drv := &recordingDriver{}
	b := &Bench{
		root:         root,
		vu:           &VU{root: root, vuid: 1, ctx: context.Background()},
		lg:           zap.NewNop(),
		drv:          drv,
		cfg:          &config.DriverConfig{DriverType: config.DriverTypeNoop},
		insertMethod: driver.InsertColumnar,
	}

	_, err := b.Insert(context.Background(), &driver.InsertRequest{
		Table:   "t",
		Method:  driver.InsertNative,
		Workers: 1,
		Source:  validInsertSource(1),
	})
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	if drv.method != driver.InsertColumnar {
		t.Fatalf("driver received method %v, want columnar", drv.method)
	}
}

func TestInsertKeepsWorkloadMethodWithoutOverride(t *testing.T) {
	installRuntimeTestRoot(t)

	drv := &recordingDriver{}
	b := &Bench{
		root: root,
		vu:   &VU{root: root, vuid: 1, ctx: context.Background()},
		lg:   zap.NewNop(),
		drv:  drv,
		cfg:  &config.DriverConfig{DriverType: config.DriverTypeNoop},
	}

	_, err := b.Insert(context.Background(), &driver.InsertRequest{
		Table:   "t",
		Method:  driver.InsertNative,
		Workers: 1,
		Source:  validInsertSource(1),
	})
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	if drv.method != driver.InsertNative {
		t.Fatalf("driver received method %v, want workload default native", drv.method)
	}
}
