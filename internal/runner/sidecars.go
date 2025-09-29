package runner

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/stroppy-io/stroppy/internal/config"
	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
	"github.com/stroppy-io/stroppy/pkg/common/shutdown"
)

type SidecarManager struct {
	sidecars []stroppy.SidecarServiceClient
}

func NewSidecarManagerFromConfig(
	logger *zap.Logger,
	config *config.Config,
) (*SidecarManager, error) {
	mgr := &SidecarManager{
		sidecars: make([]stroppy.SidecarServiceClient, 0),
	}

	for _, sc := range config.GetSideCars() {
		client, err := grpc.NewClient(sc.GetUrl(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		service := stroppy.NewSidecarServiceClient(client)

		shutdown.RegisterFn(func() {
			client.Close()
		})

		mgr.sidecars = append(mgr.sidecars, service)
	}

	return mgr, nil
}

func (m *SidecarManager) ForEachSidecar(callback func(sidecar stroppy.SidecarServiceClient) error) error {
	for _, side := range m.sidecars {
		err := callback(side)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *SidecarManager) OnStepStart(ctx context.Context, stepContext *stroppy.StepContext) error {
	return m.ForEachSidecar(func(sidecar stroppy.SidecarServiceClient) error {
		_, err := sidecar.OnStepStart(ctx, stepContext)
		if err != nil {
			return err
		}

		return nil
	})
}

func (m *SidecarManager) OnStepEnd(ctx context.Context, stepContext *stroppy.StepContext) error {
	return m.ForEachSidecar(func(sidecar stroppy.SidecarServiceClient) error {
		_, err := sidecar.OnStepEnd(ctx, stepContext)
		if err != nil {
			return err
		}

		return nil
	})
}

func (m *SidecarManager) Initialize(ctx context.Context, config *stroppy.GlobalConfig) error {
	return m.ForEachSidecar(func(sidecar stroppy.SidecarServiceClient) error {
		_, err := sidecar.Initialize(ctx, config)
		if err != nil {
			return err
		}

		return nil
	})
}

func (m *SidecarManager) Teardown(ctx context.Context, config *stroppy.GlobalConfig) error {
	return m.ForEachSidecar(func(sidecar stroppy.SidecarServiceClient) error {
		_, err := sidecar.Teardown(ctx, config)
		if err != nil {
			return err
		}

		return nil
	})
}
