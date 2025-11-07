package resource

import (
	"bytes"
	"github.com/iancoleman/strcase"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"strconv"
)

type YandexCloudProviderConfig struct {
	DefaultNetworkId        string `mapstructure:"default_network_id" validate:"required"`
	DefaultSubnetId         string `mapstructure:"default_subnet_id" validate:"required"`
	DefaultNetworkCidrBlock string `mapstructure:"default_network_cidr_block" validate:"required"`

	DefaultVmZone        string `mapstructure:"default_vm_zone" validate:"required"`
	DefaultVmPlatformId  string `mapstructure:"default_vm_platform_id" validate:"required"`
	DefaultUbuntuImageId string `mapstructure:"default_ubuntu_image_id" validate:"required"`
}

const (
	YandexCloudCrossplaneApiVersion = "vpc.yandex-cloud.jet.crossplane.io/v1alpha1"
)

// yamlKeys

const (
	ExternalNameAnnotation = "crossplane.io/external-name"
)

// dfaultValues
const (
	defaultNetworkName = "stroppy-crossplane-net"
	defaultSubnetName  = "stroppy-crossplane-subnet"
)

var (
	DefaultYandexStroppyNetworkRef = &crossplane.Ref{
		Name:      defaultNetworkName,
		Namespace: DefaultCrossplaneNamespace,
	}
	DefaultYandexStroppySubNetRef = &crossplane.Ref{
		Name:      defaultSubnetName,
		Namespace: DefaultCrossplaneNamespace,
	}
)

type YandexCloudProvider struct {
	Config *YandexCloudProviderConfig
}

func NewYandexCloudProvider(config *YandexCloudProviderConfig) *YandexCloudProvider {
	return &YandexCloudProvider{Config: config}
}

func defaultProviderConfigRef() map[string]string {
	return map[string]string{
		"name": "default",
	}
}

func (y *YandexCloudProvider) NewNetworkDef(networkIdRef *crossplane.Ref) *crossplane.ResourceDef {
	return &crossplane.ResourceDef{
		ApiVersion: YandexCloudCrossplaneApiVersion,
		Kind:       strcase.ToCamel(crossplane.YandexCloud_NETWORK.String()),
		Metadata: &crossplane.Metadata{
			Name:      networkIdRef.GetName(),
			Namespace: networkIdRef.GetNamespace(),
			Annotations: map[string]string{
				ExternalNameAnnotation: y.Config.DefaultNetworkId, // Default network ID created outside
			},
		},
		Spec: &crossplane.ResourceDef_Spec{
			DeletionPolicy:    strcase.ToCamel(crossplane.ResourceDef_Spec_ORPHAN.String()),
			ProviderConfigRef: defaultProviderConfigRef(),
			ForProvider: &crossplane.ResourceDef_Spec_YandexCloudNetwork{
				YandexCloudNetwork: &crossplane.YandexCloud_Network{
					Name: networkIdRef.GetName(),
				},
			},
		},
	}
}

func (y *YandexCloudProvider) NewSubnetDef(
	networkIdRef *crossplane.Ref,
	subnetIdRef *crossplane.Ref,
) *crossplane.ResourceDef {
	return &crossplane.ResourceDef{
		ApiVersion: YandexCloudCrossplaneApiVersion,
		Kind:       strcase.ToCamel(crossplane.YandexCloud_SUBNET.String()),
		Metadata: &crossplane.Metadata{
			Name:      subnetIdRef.GetName(),
			Namespace: subnetIdRef.GetNamespace(),
			Annotations: map[string]string{
				ExternalNameAnnotation: y.Config.DefaultSubnetId, // Default subnet ID created outside
			},
		},
		Spec: &crossplane.ResourceDef_Spec{
			DeletionPolicy:    strcase.ToCamel(crossplane.ResourceDef_Spec_ORPHAN.String()),
			ProviderConfigRef: defaultProviderConfigRef(),
			ForProvider: &crossplane.ResourceDef_Spec_YandexCloudSubnet{
				YandexCloudSubnet: &crossplane.YandexCloud_Subnet{
					Name: subnetIdRef.GetName(),
					NetworkIdRef: &crossplane.YandexCloud_Subnet_NetworkIdRef{
						Name: networkIdRef.GetName(),
					},
					V4CidrBlock: []string{y.Config.DefaultNetworkCidrBlock},
				},
			},
		},
	}
}

func (y *YandexCloudProvider) NewVmDef(
	ref *crossplane.Ref,
	machineInfo *panel.MachineInfo,
	subnetIdRef *crossplane.Ref,
	connectCredsRef *crossplane.Ref,
	script *panel.Script,
) *crossplane.ResourceDef {
	scriptBody := strconv.Quote(string(bytes.ReplaceAll(script.GetBody(), []byte("\r\n"), []byte(`\n`))))
	return &crossplane.ResourceDef{
		ApiVersion: YandexCloudCrossplaneApiVersion,
		Kind:       strcase.ToCamel(crossplane.YandexCloud_INSTANCE.String()),
		Metadata: &crossplane.Metadata{
			Name:      ref.GetName(),
			Namespace: ref.GetNamespace(),
		},
		Spec: &crossplane.ResourceDef_Spec{
			DeletionPolicy:             strcase.ToCamel(crossplane.ResourceDef_Spec_DELETE.String()),
			WriteConnectionSecretToRef: connectCredsRef,
			ProviderConfigRef:          defaultProviderConfigRef(),
			ForProvider: &crossplane.ResourceDef_Spec_YandexCloudVm{
				YandexCloudVm: &crossplane.YandexCloud_Vm{
					Name:       ref.GetName(),
					PlatformId: y.Config.DefaultVmPlatformId,
					Zone:       y.Config.DefaultVmZone,
					Resources: &crossplane.YandexCloud_Vm_Resources{
						Cores:  machineInfo.GetCores(),
						Memory: machineInfo.GetMemory(),
					},
					// yaml format shit in this block
					BootDisk: []*crossplane.YandexCloud_Vm_Disk{
						{
							InitializeParams: []*crossplane.YandexCloud_Vm_Disk_InitializeParams{
								{
									ImageId: y.Config.DefaultUbuntuImageId,
								},
							},
						},
					},
					NetworkInterface: []*crossplane.YandexCloud_Vm_NetworkInterface{
						{
							SubnetIdRef: subnetIdRef,
							Nat:         true,
						},
					},
					Metadata: map[string]string{
						// TODO: need quotes check
						"user-data": scriptBody,
					},
				},
			},
		},
	}
}
