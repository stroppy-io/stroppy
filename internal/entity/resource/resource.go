package resource

import (
	"fmt"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/protoyaml"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/timestamps"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"strings"
)

type CloudProvider interface {
	NewNetworkDef(networkIdRef *crossplane.Ref) *crossplane.ResourceDef
	NewSubnetDef(networkIdRef *crossplane.Ref, subnetIdRef *crossplane.Ref) *crossplane.ResourceDef
	NewVmDef(
		ref *crossplane.Ref,
		machineInfo *panel.MachineInfo,
		subnetIdRef *crossplane.Ref,
		connectCredsRef *crossplane.Ref,
		script *panel.Script,
	) *crossplane.ResourceDef
}

type Builder interface {
	NewDatabaseResources(
		automationId *panel.Ulid,
		database *panel.Database,
		script *panel.Script,
	) (*panel.CloudResource_TreeNode, error)
	NewWorkloadResources(
		automationId *panel.Ulid,
		workload *panel.Workload,
		script *panel.Script,
	) (*panel.CloudResource_TreeNode, error)
}

const (
	DefaultCrossplaneNamespace = "crossplane-system"
)

func ExtRefFromResourceDef(
	ref *crossplane.Ref,
	def *crossplane.ResourceDef,
) *crossplane.ExtRef {
	return &crossplane.ExtRef{
		Ref:        ref,
		ApiVersion: def.GetApiVersion(),
		Kind:       def.GetKind(),
	}
}

type builder struct {
	provider CloudProvider
}

func NewBuilder(provider CloudProvider) Builder {
	return &builder{provider: provider}
}

func NewYandexCloudBuilder(config *YandexCloudProviderConfig) Builder {
	return NewBuilder(NewYandexCloudProvider(config))
}

var (
	ErrDatabaseRunnerClusterMustHaveExactlyOneMachine = fmt.Errorf("database runner cluster must have exactly one machine")
	ErrWorkloadRunnerClusterMustHaveExactlyOneMachine = fmt.Errorf("workload runner cluster must have exactly one machine")
)

func MarshalWithReplaceOneOffs(def *crossplane.ResourceDef) (string, error) {
	// TODO: ITS BIG CRUTCH TO REPLACE FOR_PROVIDER TO ACCEPTABLE FROM K8S
	yaml, err := protoyaml.Marshal(def)
	if err != nil {
		return "", err
	}
	replacedSymbol := "NONE"
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

func (b *builder) newResourcesTree(
	networkDef *crossplane.ResourceDef,
	subnetDef *crossplane.ResourceDef,
	vmRef *crossplane.Ref,
	vmDef *crossplane.ResourceDef,
) (*panel.CloudResource_TreeNode, error) {
	vmId := ids.NewUlid()
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
		},
	}

	subnetId := ids.NewUlid()

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
		},
		Children: []*panel.CloudResource_TreeNode{vmNode},
	}

	netId := ids.NewUlid()
	networkYaml, err := MarshalWithReplaceOneOffs(networkDef)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal network def: %w", err)
	}
	return &panel.CloudResource_TreeNode{
		Id: netId,
		Resource: &panel.CloudResource{
			Id:     netId,
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

func (b *builder) NewDatabaseResources(
	automationId *panel.Ulid,
	database *panel.Database,
	script *panel.Script,
) (*panel.CloudResource_TreeNode, error) {
	if len(database.GetRunnerCluster().GetMachines()) != 1 {
		return nil, ErrDatabaseRunnerClusterMustHaveExactlyOneMachine
	}
	//lowerId := strings.ToLower(automationId.GetId())
	machineName := fmt.Sprintf("stroppy-crossplane-database-%s", automationId.GetId())
	saveSecretTo := &crossplane.Ref{
		Name:      fmt.Sprintf("%s-access-secret", machineName),
		Namespace: DefaultCrossplaneNamespace,
	}
	networkDef := b.provider.NewNetworkDef(DefaultYandexStroppyNetworkRef)
	subnetDef := b.provider.NewSubnetDef(DefaultYandexStroppyNetworkRef, DefaultYandexStroppySubNetRef)
	vmRef := &crossplane.Ref{
		Name:      machineName,
		Namespace: DefaultCrossplaneNamespace,
	}
	vmDef := b.provider.NewVmDef(
		vmRef,
		database.GetRunnerCluster().GetMachines()[0],
		DefaultYandexStroppySubNetRef,
		saveSecretTo,
		script,
	)
	return b.newResourcesTree(networkDef, subnetDef, vmRef, vmDef)
}

func (b *builder) NewWorkloadResources(
	automationId *panel.Ulid,
	workload *panel.Workload,
	script *panel.Script,
) (*panel.CloudResource_TreeNode, error) {
	if len(workload.GetRunnerCluster().GetMachines()) != 1 {
		return nil, ErrWorkloadRunnerClusterMustHaveExactlyOneMachine
	}
	lowerId := strings.ToLower(automationId.GetId())
	machineName := fmt.Sprintf("stroppy-crossplane-workload-%s", lowerId)
	saveSecretTo := &crossplane.Ref{
		Name:      fmt.Sprintf("%s-access-secret", machineName),
		Namespace: DefaultCrossplaneNamespace,
	}
	networkDef := b.provider.NewNetworkDef(DefaultYandexStroppyNetworkRef)
	subnetDef := b.provider.NewSubnetDef(DefaultYandexStroppyNetworkRef, DefaultYandexStroppySubNetRef)
	vmRef := &crossplane.Ref{
		Name:      machineName,
		Namespace: DefaultCrossplaneNamespace,
	}
	vmDef := b.provider.NewVmDef(
		vmRef,
		workload.GetRunnerCluster().GetMachines()[0],
		DefaultYandexStroppySubNetRef,
		saveSecretTo,
		script,
	)
	return b.newResourcesTree(networkDef, subnetDef, vmRef, vmDef)
}
