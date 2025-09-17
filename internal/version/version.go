package version

import (
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

type Component struct {
	Name    string
	Version string
}

func (c *Component) ToZap() []zap.Field {
	return []zap.Field{
		zap.String("version", c.Version),
	}
}

type ComponentList []Component

func (c ComponentList) DisplayVersions(logger *zap.Logger) {
	options := make([]zap.Field, 0)
	for _, comp := range c {
		options = append(options, zap.Dict(comp.Name, comp.ToZap()...))
	}

	logger.Info("Version info", options...)
}

const (
	protocolsComponentName   = "stroppy-protocol"
	stroppyCoreComponentName = "stroppy-core"
	stroppyXk6ComponentName  = "stroppy-xk6"
)

const buildTimeVersion = "unknown"

var (
	StroppyCoreVersion = buildTimeVersion //nolint: gochecknoglobals
	StroppyXk6Version  = buildTimeVersion //nolint: gochecknoglobals
)

var AllComponents = ComponentList{ //nolint: gochecknoglobals
	{
		Name:    protocolsComponentName,
		Version: stroppy.Version,
	},
	{
		Name:    stroppyCoreComponentName,
		Version: StroppyCoreVersion,
	},
	{
		Name:    stroppyXk6ComponentName,
		Version: StroppyXk6Version,
	},
}
