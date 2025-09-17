package driver

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

const (
	pluginVersion    = 1
	magicCookieKey   = "stroppy_DRIVER_PLUGIN"
	magicCookieValue = "stroppy_DRIVER_PLUGIN_HANDSHAKE"
	PluginName       = "driver_grpc"
)

var PluginHandshake = plugin.HandshakeConfig{ //nolint: gochecknoglobals // allow in shared
	// This isn't required when using VersionedPlugins
	ProtocolVersion:  pluginVersion,
	MagicCookieKey:   magicCookieKey,
	MagicCookieValue: magicCookieValue,
}

type Plugin interface {
	Initialize(ctx context.Context, runContext *stroppy.StepContext) error
	BuildTransactionsFromUnit(
		ctx context.Context,
		buildUnitContext *stroppy.UnitBuildContext,
	) (*stroppy.DriverTransactionList, error)
	BuildTransactionsFromUnitStream(
		ctx context.Context,
		buildUnitContext *stroppy.UnitBuildContext,
	) (errchan.Chan[stroppy.DriverTransaction], error)
	RunTransaction(ctx context.Context, transaction *stroppy.DriverTransaction) error
	Teardown(ctx context.Context) error
}
type SharedPlugin struct {
	plugin.Plugin
	Impl Plugin
}

func NewSharedPlugin(impl Plugin) *SharedPlugin {
	return &SharedPlugin{Impl: impl}
}

func (s SharedPlugin) GRPCServer(
	_ *plugin.GRPCBroker,
	g *grpc.Server,
) error {
	stroppy.RegisterDriverPluginServer(g, newDriverServer(s.Impl))

	return nil
}

func (s SharedPlugin) GRPCClient(
	_ context.Context,
	_ *plugin.GRPCBroker,
	conn *grpc.ClientConn,
) (interface{}, error) {
	return newDriverClient(stroppy.NewDriverPluginClient(conn)), nil
}
