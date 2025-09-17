package sidecar

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
)

const (
	pluginVersion    = 1
	magicCookieKey   = "stroppy_SIDECAR_PLUGIN"
	magicCookieValue = "stroppy_SIDECAR_PLUGIN_HANDSHAKE"
	PluginName       = "sidecar_grpc"
)

var PluginHandshake = plugin.HandshakeConfig{ //nolint: gochecknoglobals // allow in shared
	// This isn't required when using VersionedPlugins
	ProtocolVersion:  pluginVersion,
	MagicCookieKey:   magicCookieKey,
	MagicCookieValue: magicCookieValue,
}

type Plugin interface {
	Initialize(ctx context.Context, runContext *stroppy.StepContext) error
	OnStepStart(ctx context.Context, event *stroppy.StepContext) error
	OnStepEnd(ctx context.Context, event *stroppy.StepContext) error
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
	stroppy.RegisterSidecarPluginServer(g, newDriverServer(s.Impl))

	return nil
}

func (s SharedPlugin) GRPCClient(
	_ context.Context,
	_ *plugin.GRPCBroker,
	conn *grpc.ClientConn,
) (interface{}, error) {
	return newDriverClient(stroppy.NewSidecarPluginClient(conn)), nil
}
