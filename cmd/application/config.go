package application

import (
	"github.com/stroppy-io/stroppy-cloud-panel/internal/automate"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/httpserv"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
)

type Service struct {
	Logger     logger.Config             `mapstructure:"logger"`
	Auth       token.Config              `mapstructure:"auth"`
	Server     httpserv.HTTPServerConfig `mapstructure:"server"`
	K8S        automate.K8SConfig        `mapstructure:"k8s"`
	Background automate.BackgroundWorker `mapstructure:"background"`
}
type Infrastructure struct {
	Postgres postgres.Config `mapstructure:"postgres"`
}
type Config struct {
	Infra   Infrastructure `mapstructure:"infra"`
	Service Service        `mapstructure:"service"`
}
