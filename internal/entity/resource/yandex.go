package resource

import (
	"fmt"
	"github.com/iancoleman/strcase"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/timestamps"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
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
	YandexCloudVPCCrossplaneApiVersion     = "vpc.yandex-cloud.jet.crossplane.io/v1alpha1"
	YandexCloudComputeCrossplaneApiVersion = "compute.yandex-cloud.jet.crossplane.io/v1alpha1"
)

// yamlKeys

const (
	ExternalNameAnnotation = "crossplane.io/external-name"
)

// dfaultValues
const (
	defaultNetworkName     = "stroppy-crossplane-net"
	defaultSubnetName      = "stroppy-crossplane-subnet"
	defaultCreateVmWithNat = false
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

type YandexCloudBuilder struct {
	Config *YandexCloudProviderConfig
}

func NewYandexCloudBuilder(config *YandexCloudProviderConfig) *YandexCloudBuilder {
	return &YandexCloudBuilder{Config: config}
}

func defaultProviderConfigRef() map[string]string {
	return map[string]string{
		"name": "default",
	}
}

func (y *YandexCloudBuilder) newNetworkDef(networkIdRef *crossplane.Ref) *crossplane.ResourceDef {
	return &crossplane.ResourceDef{
		ApiVersion: YandexCloudVPCCrossplaneApiVersion,
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

func (y *YandexCloudBuilder) newSubnetDef(
	networkIdRef *crossplane.Ref,
	subnetIdRef *crossplane.Ref,
) *crossplane.ResourceDef {
	return &crossplane.ResourceDef{
		ApiVersion: YandexCloudVPCCrossplaneApiVersion,
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
					V4CidrBlocks: []string{y.Config.DefaultNetworkCidrBlock},
				},
			},
		},
	}
}

func (y *YandexCloudBuilder) newVmDef(
	ref *crossplane.Ref,
	machineInfo *panel.MachineInfo,
	subnetIdRef *crossplane.Ref,
	connectCredsRef *crossplane.Ref,
	script *panel.Script,
) *crossplane.ResourceDef {
	return &crossplane.ResourceDef{
		ApiVersion: YandexCloudComputeCrossplaneApiVersion,
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
					Resources: []*crossplane.YandexCloud_Vm_Resources{
						{
							Cores:  machineInfo.GetCores(),
							Memory: machineInfo.GetMemory(),
						},
					},
					// yaml format shit in this block
					BootDisk: []*crossplane.YandexCloud_Vm_Disk{
						{
							InitializeParams: []*crossplane.YandexCloud_Vm_Disk_InitializeParams{
								{
									ImageId: stringOrDefault(
										machineInfo.GetBaseImageId(),
										y.Config.DefaultUbuntuImageId,
									),
								},
							},
						},
					},
					NetworkInterface: []*crossplane.YandexCloud_Vm_NetworkInterface{
						{
							SubnetIdRef: &crossplane.OnlyNameRef{
								Name: subnetIdRef.GetName(),
							},
							Nat:       machineInfo.GetPublicIp(),
							IpAddress: machineInfo.GetStaticInternalIp(),
						},
					},
					Metadata: map[string]string{
						// TODO: need quotes check
						//"user-data": strings.ReplaceAll(string(script.GetBody()), "\n", "\\n"),
						"user-data": string(script.GetBody()),
					},
				},
			},
		},
	}
}

func (y *YandexCloudBuilder) NewSingleVmResource(
	machineName string,
	machineInfo *panel.MachineInfo,
	script *panel.Script,
) (*panel.CloudResource_TreeNode, error) {
	saveSecretTo := &crossplane.Ref{
		Name:      fmt.Sprintf("%s-access-secret", machineName),
		Namespace: DefaultCrossplaneNamespace,
	}
	networkDef := y.newNetworkDef(DefaultYandexStroppyNetworkRef)
	subnetDef := y.newSubnetDef(DefaultYandexStroppyNetworkRef, DefaultYandexStroppySubNetRef)
	vmRef := &crossplane.Ref{
		Name:      machineName,
		Namespace: DefaultCrossplaneNamespace,
	}
	vmDef := y.newVmDef(vmRef, machineInfo, DefaultYandexStroppySubNetRef, saveSecretTo, script)

	vmId := ids.NewUlid()
	subnetId := ids.NewUlid()
	networkId := ids.NewUlid()
	vmYaml, err := MarshalWithReplaceOneOffs(vmDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vm def: %w", err)
	}

	vmNode := &panel.CloudResource_TreeNode{
		Id: vmId,
		Resource: &panel.CloudResource{
			Id:     vmId,
			Timing: timestamps.NewTiming(),
			Resource: &crossplane.ResourceWithStatus{
				Ref:          vmRef,
				ResourceDef:  vmDef,
				ResourceYaml: vmYaml,
				Synced:       false,
				Ready:        false,
				ExternalId:   "",
			},
			ParentResourceId: subnetId,
		},
	}

	subnetYaml, err := MarshalWithReplaceOneOffs(subnetDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal subnet def: %w", err)
	}
	subnetNode := &panel.CloudResource_TreeNode{
		Id: subnetId,
		Resource: &panel.CloudResource{
			Id:     subnetId,
			Timing: timestamps.NewTiming(),
			Resource: &crossplane.ResourceWithStatus{
				Ref:          DefaultYandexStroppySubNetRef,
				ResourceDef:  subnetDef,
				ResourceYaml: subnetYaml,
				Synced:       false,
				Ready:        false,
				ExternalId:   "",
			},
			ParentResourceId: networkId,
		},
		Children: []*panel.CloudResource_TreeNode{vmNode},
	}

	networkYaml, err := MarshalWithReplaceOneOffs(networkDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal network def: %w", err)
	}
	return &panel.CloudResource_TreeNode{
		Id: networkId,
		Resource: &panel.CloudResource{
			Id:     networkId,
			Timing: timestamps.NewTiming(),
			Resource: &crossplane.ResourceWithStatus{
				Ref:          DefaultYandexStroppyNetworkRef,
				ResourceDef:  networkDef,
				ResourceYaml: networkYaml,
				Synced:       false,
				Ready:        false,
				ExternalId:   "",
			},
		},
		Children: []*panel.CloudResource_TreeNode{subnetNode},
	}, nil
}
