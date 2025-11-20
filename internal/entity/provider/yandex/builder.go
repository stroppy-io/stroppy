package yandex

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/samber/lo"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/defaults"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/ips"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/protoyaml"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/deployment"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ProviderConfig struct {
	DefaultNetworkId        string `mapstructure:"default_network_id" validate:"required"`
	DefaultSubnetId         string `mapstructure:"default_subnet_id" validate:"required"`
	DefaultNetworkCidrBlock string `mapstructure:"default_network_cidr_block" validate:"required"`

	DefaultVmZone       string `mapstructure:"default_vm_zone" validate:"required"`
	DefaultVmPlatformId string `mapstructure:"default_vm_platform_id" validate:"required"`
}

const (
	CloudVPCCrossplaneApiVersion     = "vpc.yandex-cloud.jet.crossplane.io/v1alpha1"
	CloudComputeCrossplaneApiVersion = "compute.yandex-cloud.jet.crossplane.io/v1alpha1"
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
	constUserDataKey       = "user-data"
)

type CloudBuilder struct {
	Config *ProviderConfig
	cidr   *net.IPNet
}

func NewCloudBuilder(config *ProviderConfig) *CloudBuilder {
	_, cidr, err := net.ParseCIDR(config.DefaultNetworkCidrBlock)
	if err != nil {
		panic("yandex.NewCloudBuilder fail parse cidr")
	}
	return &CloudBuilder{Config: config, cidr: cidr}
}

func defaultProviderConfigRef() map[string]string {
	return map[string]string{
		"name": "default",
	}
}

func resourceKindToString(kind crossplane.YandexCloud_ResourceKind) string {
	return strcase.ToCamel(kind.String())
}

func (y *CloudBuilder) UsingCidr(_ context.Context) *net.IPNet {
	return y.cidr
}

func resourceKindFromString(kind string) crossplane.YandexCloud_ResourceKind {
	knd, ok := crossplane.YandexCloud_ResourceKind_value[strings.ToUpper(kind)]
	if !ok {
		panic(fmt.Sprintf(".resourceKindFromString unknown yandex cloud resource kind: %s", kind))
	}
	return crossplane.YandexCloud_ResourceKind(knd)
}

func (y *CloudBuilder) newNetworkDef(networkIdRef *crossplane.Ref) *crossplane.ResourceDef {
	return &crossplane.ResourceDef{
		ApiVersion: CloudVPCCrossplaneApiVersion,
		Kind:       resourceKindToString(crossplane.YandexCloud_NETWORK),
		Metadata: &crossplane.Metadata{
			Name:      networkIdRef.GetName(),
			Namespace: networkIdRef.GetNamespace(),
			Annotations: map[string]string{
				ExternalNameAnnotation: y.Config.DefaultNetworkId, // Default network ID created outside
			},
		},
		Spec: &crossplane.ResourceDef_Spec{
			DeletionPolicy:    strcase.ToCamel(crossplane.CrossplaneDeletionPolicy_ORPHAN.String()),
			ProviderConfigRef: defaultProviderConfigRef(),
			ForProvider: &crossplane.ResourceDef_Spec_YandexCloudNetwork{
				YandexCloudNetwork: &crossplane.YandexCloud_Network{
					Name: networkIdRef.GetName(),
				},
			},
		},
	}
}

func (y *CloudBuilder) newSubnetDef(
	networkIdRef *crossplane.Ref,
	subnetIdRef *crossplane.Ref,
) *crossplane.ResourceDef {
	return &crossplane.ResourceDef{
		ApiVersion: CloudVPCCrossplaneApiVersion,
		Kind:       resourceKindToString(crossplane.YandexCloud_SUBNET),
		Metadata: &crossplane.Metadata{
			Name:      subnetIdRef.GetName(),
			Namespace: subnetIdRef.GetNamespace(),
			Annotations: map[string]string{
				ExternalNameAnnotation: y.Config.DefaultSubnetId, // Default subnet ID created outside
			},
		},
		Spec: &crossplane.ResourceDef_Spec{
			DeletionPolicy:    strcase.ToCamel(crossplane.CrossplaneDeletionPolicy_ORPHAN.String()),
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

var ErrEmptyInternalIp = fmt.Errorf("internal ip is empty in deployment")

func (y *CloudBuilder) newVmDef(
	ref *crossplane.Ref,
	subnetIdRef *crossplane.Ref,
	connectCredsRef *crossplane.Ref,
	vm *crossplane.Deployment_Vm,
	assignIpAddr *crossplane.Ip,
) (*crossplane.ResourceDef, error) {
	if vm.GetInternalIp().GetValue() == "" {
		return nil, ErrEmptyInternalIp
	}
	vmImageId := vm.GetStrategy().GetPrebuiltImage().GetImageId()
	var userDataYaml *UserDataYaml
	switch vm.GetStrategy().GetStrategy().(type) {
	case *crossplane.Deployment_Strategy_PrebuiltImage_:
		vmImageId = vm.GetStrategy().GetPrebuiltImage().GetImageId()
		userDataYaml = NewUserDataWithEmptyScript(vm.GetSshUser())
	case *crossplane.Deployment_Strategy_Scripting_:
		vmImageId = vm.GetStrategy().GetBaseImageId()
		userDataYaml = NewUserDataWithScript(vm.GetSshUser(), vm.GetStrategy().GetScripting())
	}
	if vmImageId == "" {
		return nil, fmt.Errorf("vm image id is empty")
	}
	machineScriptBytes, err := userDataYaml.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize machine script: %w", err)
	}
	metadata := make(map[string]string)
	metadata[constUserDataKey] = string(machineScriptBytes)

	return &crossplane.ResourceDef{
		ApiVersion: CloudComputeCrossplaneApiVersion,
		Kind:       resourceKindToString(crossplane.YandexCloud_INSTANCE),
		Metadata: &crossplane.Metadata{
			Name:      ref.GetName(),
			Namespace: ref.GetNamespace(),
		},
		Spec: &crossplane.ResourceDef_Spec{
			DeletionPolicy:             strcase.ToCamel(crossplane.CrossplaneDeletionPolicy_DELETE.String()),
			WriteConnectionSecretToRef: connectCredsRef,
			ProviderConfigRef:          defaultProviderConfigRef(),
			ForProvider: &crossplane.ResourceDef_Spec_YandexCloudVm{
				YandexCloudVm: &crossplane.YandexCloud_Vm{
					Name:       ref.GetName(),
					PlatformId: y.Config.DefaultVmPlatformId,
					Zone:       y.Config.DefaultVmZone,
					Resources: []*crossplane.YandexCloud_Vm_Resources{
						{
							Cores:  vm.GetMachineInfo().GetCores(),
							Memory: vm.GetMachineInfo().GetMemory(),
						},
					},
					// yaml format shit in this block
					BootDisk: []*crossplane.YandexCloud_Vm_Disk{
						{
							InitializeParams: []*crossplane.YandexCloud_Vm_Disk_InitializeParams{
								{
									ImageId: vmImageId,
								},
							},
						},
					},
					NetworkInterface: []*crossplane.YandexCloud_Vm_NetworkInterface{
						{
							SubnetIdRef: &crossplane.OnlyNameRef{
								Name: subnetIdRef.GetName(),
							},
							Nat:       vm.GetPublicIp(),
							IpAddress: assignIpAddr.GetValue(),
						},
					},
					Metadata: metadata,
				},
			},
		},
	}, nil
}

func (y *CloudBuilder) marshalWithReplaceOneOffs(def *crossplane.ResourceDef) (string, error) {
	yaml, err := protoyaml.Marshal(def)
	if err != nil {
		return "", err
	}
	replacedSymbol := ""
	switch def.GetSpec().GetForProvider().(type) {
	case *crossplane.ResourceDef_Spec_YandexCloudVm:
		replacedSymbol = "yandexCloudVm"
	case *crossplane.ResourceDef_Spec_YandexCloudNetwork:
		replacedSymbol = "yandexCloudNetwork"
	case *crossplane.ResourceDef_Spec_YandexCloudSubnet:
		replacedSymbol = "yandexCloudSubnet"
	}
	return strings.ReplaceAll(string(yaml), replacedSymbol, "forProvider"), nil
}

func (y *CloudBuilder) BuildVmResourceDag(
	namespace string,
	commonId *panel.Ulid,
	vm *crossplane.Deployment_Vm,
) (*deployment.VmDeploymentDagWithParams, error) {
	assignedInternalIp := &crossplane.Ip{
		Value: defaults.StringOrDefault(
			vm.GetInternalIp().GetValue(),
			lo.Must(ips.RandomIP(y.cidr)).String(),
		),
	}
	quotas := make([]*crossplane.Quota, 0)
	addQuota := func(kind crossplane.Quota_Kind) {
		quotas = append(quotas, &crossplane.Quota{
			Cloud:   crossplane.SupportedCloud_SUPPORTED_CLOUD_YANDEX,
			Kind:    kind,
			Current: 1,
		})
	}
	vmId := ids.NewUlid()

	// __WARNING__
	// Here we use vmId to generate unique names for vm
	// commonId used to generate unique names for subnet only
	// if caller of this function wants, they can set commonId to some other subnet (if they call twice)
	subnetName := fmt.Sprintf("stroppy-cloud-subnet-%s", strings.ToLower(commonId.String()))
	machineName := fmt.Sprintf("stroppy-cloud-vm-%s", strings.ToLower(vmId.String()))
	// __WARNING__

	saveSecretTo := &crossplane.Ref{
		Name:      fmt.Sprintf("%s-access-secret", machineName),
		Namespace: namespace,
	}
	networkRef := &crossplane.Ref{
		Name:      defaultNetworkName,
		Namespace: namespace,
	}
	subnetRef := &crossplane.Ref{
		Name:      subnetName,
		Namespace: namespace,
	}
	networkDef := y.newNetworkDef(networkRef)
	//addQuota(crossplane.Quota_KIND_NETWORK) // now we use one network for all vms
	subnetDef := y.newSubnetDef(networkRef, subnetRef)
	addQuota(crossplane.Quota_KIND_SUBNET)
	vmRef := &crossplane.Ref{
		Name:      machineName,
		Namespace: namespace,
	}
	vmDef, err := y.newVmDef(vmRef, subnetRef, saveSecretTo, vm, assignedInternalIp)
	if err != nil {
		return nil, err
	}
	addQuota(crossplane.Quota_KIND_VM)
	if vm.GetPublicIp() {
		addQuota(crossplane.Quota_KIND_PUBLIC_IP_ADDRESS)
	}

	subnetId := ids.NewUlid()
	networkId := ids.NewUlid()
	vmYaml, err := y.marshalWithReplaceOneOffs(vmDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vm def: %w", err)
	}
	subnetYaml, err := y.marshalWithReplaceOneOffs(subnetDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal subnet def: %w", err)
	}
	networkYaml, err := y.marshalWithReplaceOneOffs(networkDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal network def: %w", err)
	}
	dag := &crossplane.ResourceDag{
		Id: ids.NewUlid().String(),
		Nodes: []*crossplane.ResourceDag_Node{
			{
				Id: networkId.String(),
				Resource: &crossplane.Resource{
					Ref:          ids.ExtRefFromResourceDef(networkRef, networkDef),
					ResourceDef:  networkDef,
					CreatedAt:    timestamppb.Now(),
					UpdatedAt:    timestamppb.Now(),
					DeletedAt:    nil,
					ResourceYaml: networkYaml,
					Status:       crossplane.Resource_STATUS_CREATING,
					Synced:       false,
					Ready:        false,
					ExternalId:   "",
				},
			},
			{
				Id: subnetId.String(),
				Resource: &crossplane.Resource{
					Ref:          ids.ExtRefFromResourceDef(subnetRef, subnetDef),
					ResourceDef:  subnetDef,
					CreatedAt:    timestamppb.Now(),
					UpdatedAt:    timestamppb.Now(),
					DeletedAt:    nil,
					ResourceYaml: subnetYaml,
					Status:       crossplane.Resource_STATUS_CREATING,
					Synced:       false,
					Ready:        false,
					ExternalId:   "",
				},
			},
			{
				Id: vmId.String(),
				Resource: &crossplane.Resource{
					Ref:          ids.ExtRefFromResourceDef(vmRef, vmDef),
					ResourceDef:  vmDef,
					CreatedAt:    timestamppb.Now(),
					UpdatedAt:    timestamppb.Now(),
					DeletedAt:    nil,
					ResourceYaml: vmYaml,
					Status:       crossplane.Resource_STATUS_CREATING,
					Synced:       false,
					Ready:        false,
					ExternalId:   "",
				},
			},
		},
		Edges: []*crossplane.ResourceDag_Edge{
			{FromId: networkId.String(), ToId: subnetId.String()}, // network -> subnet
			{FromId: subnetId.String(), ToId: vmId.String()},      // subnet -> vm
		},
	}
	return &deployment.VmDeploymentDagWithParams{
		Dag:                dag,
		Quotas:             quotas,
		AssignedInternalIp: assignedInternalIp,
	}, nil
}
