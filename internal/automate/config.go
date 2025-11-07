package automate

import "github.com/stroppy-io/stroppy-cloud-panel/internal/entity/resource"

type CloudAutomation struct {
	YandexCloudProviderConfig resource.YandexCloudProviderConfig `mapstructure:"yandex_cloud_provider" validate:"required"`
}
type K8SConfig struct {
	KubeconfigPath string          `mapstructure:"kubeconfig_path" validate:"required"`
	Crossplane     CloudAutomation `mapstructure:"crossplane" validate:"required"`
}
