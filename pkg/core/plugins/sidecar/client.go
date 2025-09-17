package sidecar

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/hashicorp/go-plugin"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stroppy-io/stroppy-core/pkg/logger"
	"github.com/stroppy-io/stroppy-core/pkg/plugins/common"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
)

type client struct {
	lg          *zap.Logger
	protoClient stroppy.SidecarPluginClient
}

const driverClientLoggerName = "sidecar-plugin-client"

func newDriverClient(protoClient stroppy.SidecarPluginClient) *client {
	return &client{
		lg:          logger.NewStructLogger(driverClientLoggerName),
		protoClient: protoClient,
	}
}

func (d *client) Initialize(
	ctx context.Context,
	runContext *stroppy.StepContext,
) error {
	_, err := d.protoClient.Initialize(ctx, runContext)

	return err
}

func (d *client) OnStepStart(
	ctx context.Context,
	event *stroppy.StepContext,
) error {
	_, err := d.protoClient.OnStepStart(ctx, event)

	return err
}

func (d *client) OnStepEnd(
	ctx context.Context,
	event *stroppy.StepContext,
) error {
	_, err := d.protoClient.OnStepEnd(ctx, event)

	return err
}

func (d *client) Teardown(ctx context.Context) error {
	_, err := d.protoClient.Teardown(ctx, &emptypb.Empty{})

	return err
}

func ConnectToPlugin( //nolint: ireturn // need from lib
	runConfig *stroppy.RunConfig,
	lg *zap.Logger,
) (Plugin, context.CancelFunc, error) {
	logger.SetLoggerEnv(
		logger.LevelFromProtoConfig(runConfig.GetLogger().GetLogLevel()),
		logger.ModeFromProtoConfig(runConfig.GetLogger().GetLogMode()),
	)

	clientPlugin := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: PluginHandshake,
		Plugins: map[string]plugin.Plugin{
			PluginName: NewSharedPlugin(nil),
		},
		Cmd: exec.Command( //nolint: gosec // allow
			"sh",
			"-c",
			runConfig.GetDriver().GetDriverPluginPath()+" "+
				strings.Join(runConfig.GetDriver().GetDriverPluginArgs(), " "),
		),
		Logger:           common.NewLogger(lg.Named(driverClientLoggerName)),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Stderr:           os.Stderr,
		SyncStderr:       os.Stderr,
		SyncStdout:       os.Stdout,
	})

	rpcClient, err := clientPlugin.Client()
	if err != nil {
		return nil, clientPlugin.Kill, err
	}

	raw, err := rpcClient.Dispense(PluginName)
	if err != nil {
		return nil, clientPlugin.Kill, err
	}

	return raw.(Plugin), clientPlugin.Kill, nil //nolint: errcheck,forcetypeassert // allow
}
