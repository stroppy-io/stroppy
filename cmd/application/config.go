package application

import (
	"time"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/api"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/httpserv"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/crossplaneservice"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
)

type K8SConfig struct {
	KubeconfigPath    string                   `mapstructure:"kubeconfig_path" validate:"required"`
	Crossplane        resource.ResourcesConfig `mapstructure:"crossplane" validate:"required"`
	ReconcileInterval time.Duration            `mapstructure:"reconcile_interval" validate:"required"`
}

type Service struct {
	Logger     logger.Config                            `mapstructure:"logger"`
	Auth       token.Config                             `mapstructure:"auth"`
	Server     httpserv.HTTPServerConfig                `mapstructure:"server"`
	K8S        K8SConfig                                `mapstructure:"k8s"`
	Background crossplaneservice.BackgroundWorkerConfig `mapstructure:"background"`
	Automate   api.CloudAutomationConfig                `mapstructure:"automate"`
}
type Infrastructure struct {
	Postgres postgres.Config `mapstructure:"postgres"`
}
type Config struct {
	Infra   Infrastructure `mapstructure:"infra"`
	Service Service        `mapstructure:"service"`
}
