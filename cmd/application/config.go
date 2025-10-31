package application

import (
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/httpserv"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy/pkg/core/logger"
)

type K8S struct {
	KubeconfigPath string `mapstructure:"kubeconfig_path" validate:"required"`
}
type Service struct {
	Logger logger.Config             `mapstructure:"logger"`
	Auth   token.Config              `mapstructure:"auth"`
	Server httpserv.HTTPServerConfig `mapstructure:"server"`
	K8S    K8S                       `mapstructure:"k8s"`
}
type Infrastructure struct {
	Postgres postgres.Config `mapstructure:"postgres"`
}
type Config struct {
	Infra   Infrastructure `mapstructure:"infra"`
	Service Service        `mapstructure:"service"`
}
