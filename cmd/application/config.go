package application

import (
	"fmt"
	"time"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/provider/yandex"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/workflow"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/httpserv"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/crossplaneservice"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

type YandexConfig struct {
	Provider yandex.ProviderConfig `mapstructure:"provider" validate:"required"`
	Quotas   map[string]int        `mapstructure:"quotas" validate:"required"`
}

func validateYandexQuotasConfig(mp map[string]int) error {
	for kind := range mp {
		if _, ok := crossplane.Quota_Kind_value[kind]; !ok {
			return fmt.Errorf("unknown Quota_Kind: %s", kind)
		}
	}
	return nil
}

type Cloud struct {
	Yandex YandexConfig `mapstructure:"yandex" validate:"required"`
}
type K8SConfig struct {
	KubeconfigPath string `mapstructure:"kubeconfig_path" validate:"required"`
}

type CrossplaneConfig struct {
	ReconcileInterval time.Duration `mapstructure:"reconcile_interval" validate:"required"`
}
type Service struct {
	Logger     logger.Config                            `mapstructure:"logger"`
	Auth       token.Config                             `mapstructure:"auth"`
	Server     httpserv.HTTPServerConfig                `mapstructure:"server"`
	K8S        K8SConfig                                `mapstructure:"k8s"`
	Cloud      Cloud                                    `mapstructure:"cloud"`
	Background crossplaneservice.BackgroundWorkerConfig `mapstructure:"background"`
	Crossplane CrossplaneConfig                         `mapstructure:"crossplane"`
	Workflow   workflow.Config                          `mapstructure:"workflow"`
}
type Infrastructure struct {
	Postgres postgres.Config `mapstructure:"postgres"`
}
type Config struct {
	Infra   Infrastructure `mapstructure:"infra"`
	Service Service        `mapstructure:"service"`
}

func (c Config) AdditionalValidate() error {
	err := workflow.ValidateTaskRetryConfig(c.Service.Workflow.TaskRetryConfig)
	if err != nil {
		return err
	}
	err = validateYandexQuotasConfig(c.Service.Cloud.Yandex.Quotas)
	if err != nil {
		return err
	}
	return nil
}
