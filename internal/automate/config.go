package automate

import "github.com/stroppy-io/stroppy-cloud-panel/internal/entity/resource"

type K8SConfig struct {
	KubeconfigPath string                    `mapstructure:"kubeconfig_path" validate:"required"`
	Crossplane     resource.CrossplaneConfig `mapstructure:"crossplane" validate:"required"`
}
