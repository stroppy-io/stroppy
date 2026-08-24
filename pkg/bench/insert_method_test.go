package bench

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/config"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"github.com/stroppy-io/stroppy/pkg/driver/insertprogress"
	"github.com/stroppy-io/stroppy/pkg/driver/stats"
	"github.com/stroppy-io/stroppy/pkg/gen"
)

func validInsertSource() *gen.IndexedSource {
	b := gen.NewSchemaBuilder()
	id := b.Int64("id")

	return gen.NewIndexedSource(b.Build(), gen.Root{}, "test/insert-method@1", 1, 1,
		func(r gen.Row, entity uint64) error {
			r.SetInt64(id, int64(entity))

			return nil
		})
}

type recordingDriver struct {
	calls   int
	method  driver.InsertMethod
	tracker *insertprogress.Tracker
}

func (d *recordingDriver) Insert(ctx context.Context, req *driver.InsertRequest) (*stats.Query, error) {
	d.calls++
	d.method = req.Method
	d.tracker = insertprogress.FromContext(ctx)

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

func TestInsertUsesDriverFallbackWithoutMutatingRequest(t *testing.T) {
	installRuntimeTestRoot(t)

	drv := &recordingDriver{}
	b := &Bench{
		root: root,
		vu:   &VU{root: root, vuid: 1, ctx: context.Background()},
		lg:   zap.NewNop(),
		drv:  drv,
		cfg: &config.DriverConfig{
			DriverType:          config.DriverTypePostgres,
			DefaultInsertMethod: "plain_query",
		},
	}
	req := &driver.InsertRequest{Table: "t", Workers: 1, Source: validInsertSource()}

	if _, err := b.Insert(context.Background(), req); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	if drv.method != driver.InsertPlainQuery {
		t.Fatalf("driver method = %v, want plain_query", drv.method)
	}

	if req.Method != 0 {
		t.Fatalf("request method = %v, want zero", req.Method)
	}

	if drv.tracker == nil {
		t.Fatal("driver did not receive progress tracker")
	}

	if snapshot := drv.tracker.Finish(nil); snapshot.Method != "plain_query" {
		t.Fatalf("progress method = %q, want plain_query", snapshot.Method)
	}
}

func TestInsertWorkloadMethodOverridesDriverFallback(t *testing.T) {
	installRuntimeTestRoot(t)

	drv := &recordingDriver{}
	b := &Bench{
		root: root,
		vu:   &VU{root: root, vuid: 1, ctx: context.Background()},
		lg:   zap.NewNop(),
		drv:  drv,
		cfg: &config.DriverConfig{
			DriverType:          config.DriverTypePostgres,
			DefaultInsertMethod: "native",
		},
	}
	req := &driver.InsertRequest{
		Table: "t", Method: driver.InsertPlainQuery, Workers: 1, Source: validInsertSource(),
	}

	if _, err := b.Insert(context.Background(), req); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	if drv.method != driver.InsertPlainQuery {
		t.Fatalf("driver method = %v, want plain_query", drv.method)
	}

	if req.Method != driver.InsertPlainQuery {
		t.Fatalf("request method = %v, want plain_query", req.Method)
	}
}

func TestInsertRejectsMissingOrUnsupportedEffectiveMethod(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.DriverConfig
		req  *driver.InsertRequest
		err  error
	}{
		{
			name: "no default",
			cfg:  &config.DriverConfig{DriverType: config.DriverTypePostgres},
			req:  &driver.InsertRequest{Table: "t", Workers: 1, Source: validInsertSource()},
			err:  driver.ErrInsertMethodUnsupported,
		},
		{
			name: "unsupported workload override",
			cfg:  &config.DriverConfig{DriverType: config.DriverTypeMySQL, DefaultInsertMethod: "plain_bulk"},
			req: &driver.InsertRequest{
				Table: "t", Method: driver.InsertColumnar, Workers: 1, Source: validInsertSource(),
			},
			err: driver.ErrInsertMethodUnsupported,
		},
		{
			name: "shape before method resolution",
			cfg:  &config.DriverConfig{DriverType: config.DriverTypeCSV},
			req:  &driver.InsertRequest{Table: "t", Workers: 1},
			err:  driver.ErrNilInsertSource,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			installRuntimeTestRoot(t)

			drv := &recordingDriver{}
			b := &Bench{
				root: root,
				vu:   &VU{root: root, vuid: 1, ctx: context.Background()},
				lg:   zap.NewNop(),
				drv:  drv,
				cfg:  tc.cfg,
			}

			_, err := b.Insert(context.Background(), tc.req)
			if !errors.Is(err, tc.err) {
				t.Fatalf("Insert() error = %v, want %v", err, tc.err)
			}

			if drv.calls != 0 {
				t.Fatalf("driver calls = %d, want 0", drv.calls)
			}
		})
	}
}
