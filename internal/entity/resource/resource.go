package resource

import (
	"fmt"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type CloudBuilder interface {
	NewSingleVmResource(
		name string,
		machineInfo *panel.MachineInfo,
		script *panel.Script,
	) (*panel.CloudResource_TreeNode, error)
}

const (
	DefaultCrossplaneNamespace = "crossplane-system"
)

type CrossplaneConfig struct {
	YandexCloudBuilderConfig YandexCloudProviderConfig `mapstructure:"yandex_cloud_builder" validate:"required"`
}

var ErrUnsupportedCloud = fmt.Errorf("unsupported cloud")

func DispatchCloudBuilder(
	supportedCloud crossplane.SupportedCloud,
	crossplaneConfig *CrossplaneConfig,
) (CloudBuilder, error) {
	switch supportedCloud {
	case crossplane.SupportedCloud_SUPPORTED_CLOUD_YANDEX:
		return NewYandexCloudBuilder(&crossplaneConfig.YandexCloudBuilderConfig), nil
	default:
		return nil, ErrUnsupportedCloud
	}
}
