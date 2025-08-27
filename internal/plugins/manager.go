package plugins

import (
	"context"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-core/pkg/plugins/sidecar"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/shutdown"

	"github.com/stroppy-io/stroppy/internal/config"
)

type Manager struct {
	sidecars []sidecar.Plugin
}

func NewManagerFromConfig(logger *zap.Logger, config *config.Config) (*Manager, error) {
	mgr := &Manager{
		sidecars: make([]sidecar.Plugin, 0),
	}

	for _, pl := range config.GetRun().GetPlugins() {
		if pl.GetType() == stroppy.Plugin_TYPE_SIDECAR {
			plug, cancel, err := sidecar.ConnectToPlugin(config.GetRun(), logger)
			if err != nil {
				return nil, err
			}

			shutdown.RegisterFn(cancel)

			mgr.sidecars = append(mgr.sidecars, plug)
		}
	}

	return mgr, nil
}

type initializable interface {
	Initialize(ctx context.Context, runContext *stroppy.StepContext) error
	Teardown(ctx context.Context) error
}

func (m *Manager) mergeInitializable() []initializable {
	res := make([]initializable, 0)
	for _, pl := range m.sidecars {
		res = append(res, pl)
	}

	return res
}

func (m *Manager) ForEachSidecar(callback func(sidecar.Plugin) error) error {
	for _, side := range m.sidecars {
		err := callback(side)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) Initialize(ctx context.Context, runContext *stroppy.StepContext) error {
	initable := m.mergeInitializable()
	for _, ini := range initable {
		err := ini.Initialize(ctx, runContext)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) Teardown(ctx context.Context) error {
	initable := m.mergeInitializable()
	for _, ini := range initable {
		err := ini.Teardown(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}
