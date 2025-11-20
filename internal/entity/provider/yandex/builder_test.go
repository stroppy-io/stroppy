package yandex

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

// Test helper to create a basic ProviderConfig
func getTestProviderConfig() *ProviderConfig {
	return &ProviderConfig{
		DefaultNetworkId:        "test-network-id",
		DefaultSubnetId:         "test-subnet-id",
		DefaultNetworkCidrBlock: "10.0.0.0/24",
		DefaultVmZone:           "ru-central1-a",
		DefaultVmPlatformId:     "standard-v2",
	}
}

// Test helper to create a basic VM
func getTestVm() *crossplane.Deployment_Vm {
	return &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: "10.0.0.10"},
		PublicIp:   true,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  2,
			Memory: 4,
		},
		Strategy: &crossplane.Deployment_Strategy{
			BaseImageId: "test-base-image-id",
			Strategy: &crossplane.Deployment_Strategy_PrebuiltImage_{
				PrebuiltImage: &crossplane.Deployment_Strategy_PrebuiltImage{
					ImageId: "test-prebuilt-image-id",
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name:              "test-user",
			SshAuthorizedKeys: []string{"ssh-rsa AAAAB3NzaC1yc2E..."},
		},
	}
}

func TestNewCloudBuilder_ValidConfig(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	require.NotNil(t, builder)
	require.NotNil(t, builder.Config)
	require.NotNil(t, builder.cidr)
	require.Equal(t, config, builder.Config)
}

func TestNewCloudBuilder_InvalidCIDR(t *testing.T) {
	config := &ProviderConfig{
		DefaultNetworkId:        "test-network-id",
		DefaultSubnetId:         "test-subnet-id",
		DefaultNetworkCidrBlock: "invalid-cidr",
		DefaultVmZone:           "ru-central1-a",
		DefaultVmPlatformId:     "standard-v2",
	}

	require.Panics(t, func() {
		NewCloudBuilder(config)
	})
}

func TestCloudBuilder_UsingCidr(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	ctx := context.Background()
	cidr := builder.UsingCidr(ctx)

	require.NotNil(t, cidr)
	require.Equal(t, "10.0.0.0/24", cidr.String())
}

func TestResourceKindToString_Network(t *testing.T) {
	result := resourceKindToString(crossplane.YandexCloud_NETWORK)
	require.Equal(t, "Network", result)
}

func TestResourceKindToString_Subnet(t *testing.T) {
	result := resourceKindToString(crossplane.YandexCloud_SUBNET)
	require.Equal(t, "Subnet", result)
}

func TestResourceKindToString_Instance(t *testing.T) {
	result := resourceKindToString(crossplane.YandexCloud_INSTANCE)
	require.Equal(t, "Instance", result)
}

func TestResourceKindFromString_Network(t *testing.T) {
	result := resourceKindFromString("NETWORK")
	require.Equal(t, crossplane.YandexCloud_NETWORK, result)
}

func TestResourceKindFromString_Subnet(t *testing.T) {
	result := resourceKindFromString("SUBNET")
	require.Equal(t, crossplane.YandexCloud_SUBNET, result)
}

func TestResourceKindFromString_Instance(t *testing.T) {
	result := resourceKindFromString("INSTANCE")
	require.Equal(t, crossplane.YandexCloud_INSTANCE, result)
}

func TestResourceKindFromString_Unknown(t *testing.T) {
	require.Panics(t, func() {
		resourceKindFromString("UNKNOWN")
	})
}

func TestCloudBuilder_newNetworkDef(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	networkRef := &crossplane.Ref{
		Name:      "test-network",
		Namespace: "test-namespace",
	}

	networkDef := builder.newNetworkDef(networkRef)

	require.NotNil(t, networkDef)
	require.Equal(t, CloudVPCCrossplaneApiVersion, networkDef.ApiVersion)
	require.Equal(t, "Network", networkDef.Kind)
	require.Equal(t, networkRef.Name, networkDef.Metadata.Name)
	require.Equal(t, networkRef.Namespace, networkDef.Metadata.Namespace)
	require.Equal(t, config.DefaultNetworkId, networkDef.Metadata.Annotations[ExternalNameAnnotation])
	require.Equal(t, "Orphan", networkDef.Spec.DeletionPolicy)
	require.Equal(t, "default", networkDef.Spec.ProviderConfigRef["name"])

	require.NotNil(t, networkDef.Spec.GetYandexCloudNetwork())
	require.Equal(t, networkRef.Name, networkDef.Spec.GetYandexCloudNetwork().Name)
}

func TestCloudBuilder_newSubnetDef(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	networkRef := &crossplane.Ref{
		Name:      "test-network",
		Namespace: "test-namespace",
	}
	subnetRef := &crossplane.Ref{
		Name:      "test-subnet",
		Namespace: "test-namespace",
	}

	subnetDef := builder.newSubnetDef(networkRef, subnetRef)

	require.NotNil(t, subnetDef)
	require.Equal(t, CloudVPCCrossplaneApiVersion, subnetDef.ApiVersion)
	require.Equal(t, "Subnet", subnetDef.Kind)
	require.Equal(t, subnetRef.Name, subnetDef.Metadata.Name)
	require.Equal(t, subnetRef.Namespace, subnetDef.Metadata.Namespace)
	require.Equal(t, config.DefaultSubnetId, subnetDef.Metadata.Annotations[ExternalNameAnnotation])
	require.Equal(t, "Orphan", subnetDef.Spec.DeletionPolicy)
	require.Equal(t, "default", subnetDef.Spec.ProviderConfigRef["name"])

	require.NotNil(t, subnetDef.Spec.GetYandexCloudSubnet())
	subnet := subnetDef.Spec.GetYandexCloudSubnet()
	require.Equal(t, subnetRef.Name, subnet.Name)
	require.Equal(t, networkRef.Name, subnet.NetworkIdRef.Name)
	require.Equal(t, []string{config.DefaultNetworkCidrBlock}, subnet.V4CidrBlocks)
}

func TestCloudBuilder_newVmDef_PrebuiltImage(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	vmRef := &crossplane.Ref{
		Name:      "test-vm",
		Namespace: "test-namespace",
	}
	subnetRef := &crossplane.Ref{
		Name:      "test-subnet",
		Namespace: "test-namespace",
	}
	connectCredsRef := &crossplane.Ref{
		Name:      "test-vm-secret",
		Namespace: "test-namespace",
	}

	vm := getTestVm()
	assignIpAddr := &crossplane.Ip{Value: "10.0.0.10"}

	vmDef, err := builder.newVmDef(vmRef, subnetRef, connectCredsRef, vm, assignIpAddr)

	require.NoError(t, err)
	require.NotNil(t, vmDef)

	require.Equal(t, CloudComputeCrossplaneApiVersion, vmDef.ApiVersion)
	require.Equal(t, "Instance", vmDef.Kind)
	require.Equal(t, vmRef.Name, vmDef.Metadata.Name)
	require.Equal(t, vmRef.Namespace, vmDef.Metadata.Namespace)
	require.Equal(t, "Delete", vmDef.Spec.DeletionPolicy)
	require.Equal(t, "default", vmDef.Spec.ProviderConfigRef["name"])
	require.Equal(t, connectCredsRef, vmDef.Spec.WriteConnectionSecretToRef)

	require.NotNil(t, vmDef.Spec.GetYandexCloudVm())
	vmSpec := vmDef.Spec.GetYandexCloudVm()
	require.Equal(t, vmRef.Name, vmSpec.Name)
	require.Equal(t, config.DefaultVmPlatformId, vmSpec.PlatformId)
	require.Equal(t, config.DefaultVmZone, vmSpec.Zone)
	require.Equal(t, vm.MachineInfo.Cores, vmSpec.Resources[0].Cores)
	require.Equal(t, vm.MachineInfo.Memory, vmSpec.Resources[0].Memory)
	require.Equal(t, subnetRef.Name, vmSpec.NetworkInterface[0].SubnetIdRef.Name)
	require.Equal(t, vm.PublicIp, vmSpec.NetworkInterface[0].Nat)
	require.Equal(t, assignIpAddr.Value, vmSpec.NetworkInterface[0].IpAddress)
	require.Contains(t, vmSpec.Metadata, constUserDataKey)
}

func TestCloudBuilder_newVmDef_ScriptingStrategy(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	vmRef := &crossplane.Ref{
		Name:      "test-vm",
		Namespace: "test-namespace",
	}
	subnetRef := &crossplane.Ref{
		Name:      "test-subnet",
		Namespace: "test-namespace",
	}
	connectCredsRef := &crossplane.Ref{
		Name:      "test-vm-secret",
		Namespace: "test-namespace",
	}

	vm := &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: "10.0.0.11"},
		PublicIp:   false,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  4,
			Memory: 8,
		},
		Strategy: &crossplane.Deployment_Strategy{
			BaseImageId: "base-image-id",
			Strategy: &crossplane.Deployment_Strategy_Scripting_{
				Scripting: &crossplane.Deployment_Strategy_Scripting{
					Cmd:     "echo 'test'",
					Workdir: "/tmp",
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name:              "admin",
			SshAuthorizedKeys: []string{"ssh-rsa key"},
		},
	}
	assignIpAddr := &crossplane.Ip{Value: "10.0.0.11"}

	vmDef, err := builder.newVmDef(vmRef, subnetRef, connectCredsRef, vm, assignIpAddr)

	require.NoError(t, err)
	require.NotNil(t, vmDef)

	require.Equal(t, CloudComputeCrossplaneApiVersion, vmDef.ApiVersion)
	require.Equal(t, "Instance", vmDef.Kind)
	require.Contains(t, vmDef.Spec.GetYandexCloudVm().Metadata, constUserDataKey)
}

func TestCloudBuilder_newVmDef_EmptyInternalIP(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	vmRef := &crossplane.Ref{
		Name:      "test-vm",
		Namespace: "test-namespace",
	}
	subnetRef := &crossplane.Ref{
		Name:      "test-subnet",
		Namespace: "test-namespace",
	}
	connectCredsRef := &crossplane.Ref{
		Name:      "test-vm-secret",
		Namespace: "test-namespace",
	}

	vm := &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: ""},
		PublicIp:   true,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  2,
			Memory: 4,
		},
		Strategy: &crossplane.Deployment_Strategy{
			Strategy: &crossplane.Deployment_Strategy_PrebuiltImage_{
				PrebuiltImage: &crossplane.Deployment_Strategy_PrebuiltImage{
					ImageId: "test-image-id",
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name: "test-user",
		},
	}
	assignIpAddr := &crossplane.Ip{Value: "10.0.0.12"}

	vmDef, err := builder.newVmDef(vmRef, subnetRef, connectCredsRef, vm, assignIpAddr)

	require.Error(t, err)
	require.Contains(t, err.Error(), "internal ip is empty")
	require.Nil(t, vmDef)
}

func TestCloudBuilder_newVmDef_EmptyImageID(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	vmRef := &crossplane.Ref{
		Name:      "test-vm",
		Namespace: "test-namespace",
	}
	subnetRef := &crossplane.Ref{
		Name:      "test-subnet",
		Namespace: "test-namespace",
	}
	connectCredsRef := &crossplane.Ref{
		Name:      "test-vm-secret",
		Namespace: "test-namespace",
	}

	vm := &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: "10.0.0.13"},
		PublicIp:   true,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  2,
			Memory: 4,
		},
		Strategy: &crossplane.Deployment_Strategy{
			Strategy: &crossplane.Deployment_Strategy_PrebuiltImage_{
				PrebuiltImage: &crossplane.Deployment_Strategy_PrebuiltImage{
					ImageId: "",
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name: "test-user",
		},
	}
	assignIpAddr := &crossplane.Ip{Value: "10.0.0.13"}

	vmDef, err := builder.newVmDef(vmRef, subnetRef, connectCredsRef, vm, assignIpAddr)

	require.Error(t, err)
	require.Contains(t, err.Error(), "vm image id is empty")
	require.Nil(t, vmDef)
}

func TestCloudBuilder_marshalWithReplaceOneOffs_Network(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	resourceDef := builder.newNetworkDef(&crossplane.Ref{
		Name:      "test-network",
		Namespace: "test-namespace",
	})

	yaml, err := builder.marshalWithReplaceOneOffs(resourceDef)
	require.NoError(t, err)
	require.NotEmpty(t, yaml)

	require.Contains(t, yaml, "forProvider")
	require.NotContains(t, yaml, "yandexCloudNetwork")
}

func TestCloudBuilder_marshalWithReplaceOneOffs_Subnet(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	resourceDef := builder.newSubnetDef(
		&crossplane.Ref{Name: "test-network", Namespace: "test-namespace"},
		&crossplane.Ref{Name: "test-subnet", Namespace: "test-namespace"},
	)

	yaml, err := builder.marshalWithReplaceOneOffs(resourceDef)
	require.NoError(t, err)
	require.NotEmpty(t, yaml)

	require.Contains(t, yaml, "forProvider")
	require.NotContains(t, yaml, "yandexCloudSubnet")
}

func TestCloudBuilder_BuildVmResourceDag_WithPublicIP(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	namespace := "test-namespace"
	commonId := ids.NewUlid()
	vm := getTestVm()

	result, err := builder.BuildVmResourceDag(namespace, commonId, vm)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Check quotas - should have SUBNET, VM, PUBLIC_IP_ADDRESS
	require.Len(t, result.Quotas, 3)
	hasPublicIp := false
	for _, q := range result.Quotas {
		if q.Kind == crossplane.Quota_KIND_PUBLIC_IP_ADDRESS {
			hasPublicIp = true
			break
		}
	}
	require.True(t, hasPublicIp, "Expected PUBLIC_IP_ADDRESS quota")

	// Check assigned internal IP
	require.NotNil(t, result.AssignedInternalIp)
	require.NotEmpty(t, result.AssignedInternalIp.Value)
	ip := net.ParseIP(result.AssignedInternalIp.Value)
	require.NotNil(t, ip, "AssignedInternalIp should be a valid IP address")
	require.True(t, builder.cidr.Contains(ip), "AssignedInternalIp should be within CIDR range")

	// Check DAG structure
	require.NotNil(t, result.Dag)
	require.NotEmpty(t, result.Dag.Id)
	require.Len(t, result.Dag.Nodes, 3) // network, subnet, vm
	require.Len(t, result.Dag.Edges, 2) // network->subnet, subnet->vm

	// Verify node types
	var networkNode, subnetNode, vmNode *crossplane.ResourceDag_Node
	for _, node := range result.Dag.Nodes {
		require.NotNil(t, node.Resource)
		require.NotNil(t, node.Resource.ResourceDef)

		switch node.Resource.ResourceDef.Kind {
		case "Network":
			networkNode = node
		case "Subnet":
			subnetNode = node
		case "Instance":
			vmNode = node
		}
	}

	require.NotNil(t, networkNode, "DAG should contain a Network node")
	require.NotNil(t, subnetNode, "DAG should contain a Subnet node")
	require.NotNil(t, vmNode, "DAG should contain an Instance node")

	// Verify network node
	require.Equal(t, CloudVPCCrossplaneApiVersion, networkNode.Resource.ResourceDef.ApiVersion)
	require.Contains(t, networkNode.Resource.ResourceDef.Metadata.Name, defaultNetworkName)
	require.NotEmpty(t, networkNode.Resource.ResourceYaml)
	require.Equal(t, crossplane.Resource_STATUS_CREATING, networkNode.Resource.Status)
	require.False(t, networkNode.Resource.Synced)
	require.False(t, networkNode.Resource.Ready)

	// Verify subnet node
	require.Equal(t, CloudVPCCrossplaneApiVersion, subnetNode.Resource.ResourceDef.ApiVersion)
	require.Contains(t, subnetNode.Resource.ResourceDef.Metadata.Name, "stroppy-cloud-subnet")
	require.Contains(t, subnetNode.Resource.ResourceDef.Metadata.Name, strings.ToLower(commonId.String()))
	require.NotEmpty(t, subnetNode.Resource.ResourceYaml)
	require.Equal(t, crossplane.Resource_STATUS_CREATING, subnetNode.Resource.Status)

	// Verify VM node
	require.Equal(t, CloudComputeCrossplaneApiVersion, vmNode.Resource.ResourceDef.ApiVersion)
	require.Contains(t, vmNode.Resource.ResourceDef.Metadata.Name, "stroppy-cloud-vm")
	require.NotEmpty(t, vmNode.Resource.ResourceYaml)
	require.Equal(t, crossplane.Resource_STATUS_CREATING, vmNode.Resource.Status)

	// Verify edges (network -> subnet -> vm)
	networkToSubnet := false
	subnetToVm := false
	for _, edge := range result.Dag.Edges {
		if edge.FromId == networkNode.Id && edge.ToId == subnetNode.Id {
			networkToSubnet = true
		}
		if edge.FromId == subnetNode.Id && edge.ToId == vmNode.Id {
			subnetToVm = true
		}
	}
	require.True(t, networkToSubnet, "Should have edge from network to subnet")
	require.True(t, subnetToVm, "Should have edge from subnet to VM")

	// Verify all resources have correct namespace
	for _, node := range result.Dag.Nodes {
		require.Equal(t, namespace, node.Resource.ResourceDef.Metadata.Namespace)
	}

	// Verify YAML doesn't contain oneOf field names
	require.NotContains(t, networkNode.Resource.ResourceYaml, "yandexCloudNetwork")
	require.NotContains(t, subnetNode.Resource.ResourceYaml, "yandexCloudSubnet")
	require.NotContains(t, vmNode.Resource.ResourceYaml, "yandexCloudVm")
}

func TestCloudBuilder_BuildVmResourceDag_WithoutPublicIP(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	namespace := "test-namespace"
	commonId := ids.NewUlid()
	vm := &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: "10.0.0.20"},
		PublicIp:   false,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  2,
			Memory: 4,
		},
		Strategy: &crossplane.Deployment_Strategy{
			Strategy: &crossplane.Deployment_Strategy_PrebuiltImage_{
				PrebuiltImage: &crossplane.Deployment_Strategy_PrebuiltImage{
					ImageId: "test-image-id",
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name:              "test-user",
			SshAuthorizedKeys: []string{"ssh-rsa key"},
		},
	}

	result, err := builder.BuildVmResourceDag(namespace, commonId, vm)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Check quotas - should have SUBNET, VM (no PUBLIC_IP_ADDRESS)
	require.Len(t, result.Quotas, 2)
	for _, q := range result.Quotas {
		require.NotEqual(t, crossplane.Quota_KIND_PUBLIC_IP_ADDRESS, q.Kind, "Should not have PUBLIC_IP_ADDRESS quota")
	}

	// Check DAG structure
	require.NotNil(t, result.Dag)
	require.Len(t, result.Dag.Nodes, 3)
	require.Len(t, result.Dag.Edges, 2)
}

func TestCloudBuilder_BuildVmResourceDag_WithScriptingStrategy(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	namespace := "test-namespace"
	commonId := ids.NewUlid()
	vm := &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: "10.0.0.30"},
		PublicIp:   true,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  4,
			Memory: 8,
		},
		Strategy: &crossplane.Deployment_Strategy{
			BaseImageId: "base-image-id",
			Strategy: &crossplane.Deployment_Strategy_Scripting_{
				Scripting: &crossplane.Deployment_Strategy_Scripting{
					Cmd:     "bash script.sh",
					Workdir: "/home/user",
					FilesToWrite: []*crossplane.FsFile{
						{
							Path:    "script.sh",
							Content: []byte("#!/bin/bash\necho 'Hello World'"),
						},
					},
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name:              "admin",
			SshAuthorizedKeys: []string{"ssh-rsa key"},
		},
	}

	result, err := builder.BuildVmResourceDag(namespace, commonId, vm)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Quotas, 3)
	require.Len(t, result.Dag.Nodes, 3)
	require.Len(t, result.Dag.Edges, 2)
}

func TestCloudBuilder_BuildVmResourceDag_MultipleCallsUseSameNetwork(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	namespace := "test-namespace"
	commonId1 := ids.NewUlid()
	commonId2 := ids.NewUlid()

	vm1 := getTestVm()
	vm2 := getTestVm()
	vm2.InternalIp.Value = "10.0.0.11"

	result1, err1 := builder.BuildVmResourceDag(namespace, commonId1, vm1)
	require.NoError(t, err1)

	result2, err2 := builder.BuildVmResourceDag(namespace, commonId2, vm2)
	require.NoError(t, err2)

	// Both should use the same network name
	var network1Name, network2Name string
	for _, node := range result1.Dag.Nodes {
		if node.Resource.ResourceDef.Kind == "Network" {
			network1Name = node.Resource.ResourceDef.Metadata.Name
		}
	}
	for _, node := range result2.Dag.Nodes {
		if node.Resource.ResourceDef.Kind == "Network" {
			network2Name = node.Resource.ResourceDef.Metadata.Name
		}
	}

	require.Equal(t, network1Name, network2Name, "Both DAGs should use the same network name")
	require.Equal(t, defaultNetworkName, network1Name)

	// But subnets should be different (based on commonId)
	var subnet1Name, subnet2Name string
	for _, node := range result1.Dag.Nodes {
		if node.Resource.ResourceDef.Kind == "Subnet" {
			subnet1Name = node.Resource.ResourceDef.Metadata.Name
		}
	}
	for _, node := range result2.Dag.Nodes {
		if node.Resource.ResourceDef.Kind == "Subnet" {
			subnet2Name = node.Resource.ResourceDef.Metadata.Name
		}
	}

	require.NotEqual(t, subnet1Name, subnet2Name, "Different commonIds should result in different subnet names")
}

func TestDefaultProviderConfigRef(t *testing.T) {
	ref := defaultProviderConfigRef()
	require.NotNil(t, ref)
	require.Equal(t, "default", ref["name"])
}

// Tests for scripting mechanism

func TestNewUserDataWithEmptyScript(t *testing.T) {
	sshUser := &crossplane.SshUser{
		Name:              "testuser",
		SshAuthorizedKeys: []string{"ssh-rsa AAAAB3NzaC1yc2E... user@host"},
	}

	userData := NewUserDataWithEmptyScript(sshUser)

	require.NotNil(t, userData)
	require.Equal(t, "no", userData.SSHPwauth)
	require.Len(t, userData.Users, 1)
	require.Equal(t, "testuser", userData.Users[0].Name)
	require.Equal(t, "/bin/bash", userData.Users[0].Shell)
	require.Equal(t, "ALL=(ALL) NOPASSWD:ALL", userData.Users[0].Sudo)
	require.Equal(t, sshUser.SshAuthorizedKeys, userData.Users[0].SSHAuthorizedKeys)
	require.Nil(t, userData.Runcmd)
	require.False(t, userData.Datasource.Ec2.StrictID)
}

func TestNewUserDataWithEmptyScript_DefaultUsername(t *testing.T) {
	sshUser := &crossplane.SshUser{
		Name:              "", // Empty name should use default
		SshAuthorizedKeys: []string{"ssh-rsa key"},
	}

	userData := NewUserDataWithEmptyScript(sshUser)

	require.NotNil(t, userData)
	require.Len(t, userData.Users, 1)
	require.Equal(t, "st-t-postgres", userData.Users[0].Name) // Default username
}

func TestNewUserDataWithScript(t *testing.T) {
	sshUser := &crossplane.SshUser{
		Name:              "admin",
		SshAuthorizedKeys: []string{"ssh-rsa AAAAB3NzaC1yc2E... admin@host"},
	}

	script := &crossplane.Deployment_Strategy_Scripting{
		Cmd:     "bash /home/admin/setup.sh",
		Workdir: "/home/admin",
		FilesToWrite: []*crossplane.FsFile{
			{
				Path:    "/home/admin/setup.sh",
				Content: []byte("#!/bin/bash\necho 'Installing packages...'\napt-get update\napt-get install -y postgresql"),
			},
		},
	}

	userData := NewUserDataWithScript(sshUser, script)

	require.NotNil(t, userData)
	require.Equal(t, "no", userData.SSHPwauth)
	require.Len(t, userData.Users, 1)
	require.Equal(t, "admin", userData.Users[0].Name)
	require.Equal(t, sshUser.SshAuthorizedKeys, userData.Users[0].SSHAuthorizedKeys)

	// Check runcmd structure
	require.NotNil(t, userData.Runcmd)
	require.Len(t, userData.Runcmd, 1)
	require.Len(t, userData.Runcmd[0], 4)
	require.Equal(t, "su", userData.Runcmd[0][0])
	require.Equal(t, "admin", userData.Runcmd[0][1])
	require.Equal(t, "-c", userData.Runcmd[0][2])

	// The command should contain base64 decoding and script execution
	cmd := userData.Runcmd[0][3]
	require.Contains(t, cmd, "base64 -d")
	require.Contains(t, cmd, "stroppy-cloud-script.sh")
	require.Contains(t, cmd, "chmod +x")
}

func TestNewUserDataWithScript_MultipleFiles(t *testing.T) {
	sshUser := &crossplane.SshUser{
		Name:              "deployer",
		SshAuthorizedKeys: []string{"ssh-rsa key1", "ssh-rsa key2"},
	}

	script := &crossplane.Deployment_Strategy_Scripting{
		Cmd:     "./deploy.sh",
		Workdir: "/opt/app",
		FilesToWrite: []*crossplane.FsFile{
			{
				Path:    "deploy.sh",
				Content: []byte("#!/bin/bash\nsource config.env\n./install.sh"),
			},
			{
				Path:    "config.env",
				Content: []byte("DB_HOST=localhost\nDB_PORT=5432"),
			},
			{
				Path:    "install.sh",
				Content: []byte("#!/bin/bash\necho 'Installing application...'"),
			},
		},
	}

	userData := NewUserDataWithScript(sshUser, script)

	require.NotNil(t, userData)
	require.Len(t, userData.Users[0].SSHAuthorizedKeys, 2)
	require.NotNil(t, userData.Runcmd)
}

func TestNewUserDataWithScript_EmptyFilesToWrite(t *testing.T) {
	sshUser := &crossplane.SshUser{
		Name:              "runner",
		SshAuthorizedKeys: []string{"ssh-rsa key"},
	}

	script := &crossplane.Deployment_Strategy_Scripting{
		Cmd:          "echo 'Hello World'",
		Workdir:      "/tmp",
		FilesToWrite: []*crossplane.FsFile{}, // Empty files
	}

	userData := NewUserDataWithScript(sshUser, script)

	require.NotNil(t, userData)
	require.NotNil(t, userData.Runcmd)
}

func TestUserDataYaml_Serialize(t *testing.T) {
	userData := &UserDataYaml{
		Datasource: userDataYamlDataSource{
			Ec2: userDataYamlDataSourceEc2{
				StrictID: false,
			},
		},
		SSHPwauth: "no",
		Users: []userDataYamlUsers{
			{
				Name:              "testuser",
				Sudo:              "ALL=(ALL) NOPASSWD:ALL",
				Shell:             "/bin/bash",
				SSHAuthorizedKeys: []string{"ssh-rsa key1"},
			},
		},
		Runcmd: [][]string{
			{"echo", "test"},
		},
	}

	serialized, err := userData.Serialize()

	require.NoError(t, err)
	require.NotEmpty(t, serialized)

	// Check that newlines are escaped with \n
	require.Contains(t, string(serialized), "\\n")

	// Check that YAML structure is present (PascalCase in YAML)
	require.Contains(t, string(serialized), "Datasource")
	require.Contains(t, string(serialized), "SSHPwauth")
	require.Contains(t, string(serialized), "Users")
	require.Contains(t, string(serialized), "testuser")
	require.Contains(t, string(serialized), "Runcmd")
}

func TestUserDataYaml_Serialize_ComplexStructure(t *testing.T) {
	userData := &UserDataYaml{
		Datasource: userDataYamlDataSource{
			Ec2: userDataYamlDataSourceEc2{
				StrictID: false,
			},
		},
		SSHPwauth: "no",
		Users: []userDataYamlUsers{
			{
				Name:              "admin",
				Sudo:              "ALL=(ALL) NOPASSWD:ALL",
				Shell:             "/bin/bash",
				SSHAuthorizedKeys: []string{"key1", "key2", "key3"},
			},
		},
		Runcmd: [][]string{
			{"su", "admin", "-c", "echo test | base64 -d > script.sh && chmod +x script.sh && ./script.sh"},
			{"systemctl", "restart", "nginx"},
		},
	}

	serialized, err := userData.Serialize()

	require.NoError(t, err)
	require.NotEmpty(t, serialized)

	// Verify multi-line YAML is escaped
	require.Contains(t, string(serialized), "\\n")
	require.Contains(t, string(serialized), "SSHAuthorizedKeys")
}

func TestCloudBuilder_newVmDef_ScriptingUserDataContent(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	vmRef := &crossplane.Ref{
		Name:      "test-vm",
		Namespace: "test-namespace",
	}
	subnetRef := &crossplane.Ref{
		Name:      "test-subnet",
		Namespace: "test-namespace",
	}
	connectCredsRef := &crossplane.Ref{
		Name:      "test-vm-secret",
		Namespace: "test-namespace",
	}

	vm := &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: "10.0.0.50"},
		PublicIp:   true,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  4,
			Memory: 8,
		},
		Strategy: &crossplane.Deployment_Strategy{
			BaseImageId: "ubuntu-2004-lts",
			Strategy: &crossplane.Deployment_Strategy_Scripting_{
				Scripting: &crossplane.Deployment_Strategy_Scripting{
					Cmd:     "bash setup.sh",
					Workdir: "/home/deploy",
					FilesToWrite: []*crossplane.FsFile{
						{
							Path:    "setup.sh",
							Content: []byte("#!/bin/bash\necho 'Running setup'\napt-get update"),
						},
					},
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name:              "deploy",
			SshAuthorizedKeys: []string{"ssh-rsa AAAAB3... deploy@host"},
		},
	}
	assignIpAddr := &crossplane.Ip{Value: "10.0.0.50"}

	vmDef, err := builder.newVmDef(vmRef, subnetRef, connectCredsRef, vm, assignIpAddr)

	require.NoError(t, err)
	require.NotNil(t, vmDef)

	vmSpec := vmDef.Spec.GetYandexCloudVm()
	require.NotNil(t, vmSpec)

	// Check that user-data metadata exists
	require.Contains(t, vmSpec.Metadata, constUserDataKey)
	userDataStr := vmSpec.Metadata[constUserDataKey]
	require.NotEmpty(t, userDataStr)

	// Verify user-data contains expected cloud-init structure
	require.Contains(t, userDataStr, "Datasource")
	require.Contains(t, userDataStr, "SSHPwauth")
	require.Contains(t, userDataStr, "Users")
	require.Contains(t, userDataStr, "deploy") // username
	require.Contains(t, userDataStr, "Runcmd")
	require.Contains(t, userDataStr, "stroppy-cloud-script.sh")
	require.Contains(t, userDataStr, "base64 -d")

	// Check that newlines are escaped
	require.Contains(t, userDataStr, "\\n")
}

func TestCloudBuilder_newVmDef_PrebuiltImageUserDataContent(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	vmRef := &crossplane.Ref{
		Name:      "test-vm",
		Namespace: "test-namespace",
	}
	subnetRef := &crossplane.Ref{
		Name:      "test-subnet",
		Namespace: "test-namespace",
	}
	connectCredsRef := &crossplane.Ref{
		Name:      "test-vm-secret",
		Namespace: "test-namespace",
	}

	vm := &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: "10.0.0.60"},
		PublicIp:   false,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  2,
			Memory: 4,
		},
		Strategy: &crossplane.Deployment_Strategy{
			Strategy: &crossplane.Deployment_Strategy_PrebuiltImage_{
				PrebuiltImage: &crossplane.Deployment_Strategy_PrebuiltImage{
					ImageId: "prebuilt-postgres-image",
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name:              "postgres",
			SshAuthorizedKeys: []string{"ssh-rsa key"},
		},
	}
	assignIpAddr := &crossplane.Ip{Value: "10.0.0.60"}

	vmDef, err := builder.newVmDef(vmRef, subnetRef, connectCredsRef, vm, assignIpAddr)

	require.NoError(t, err)
	require.NotNil(t, vmDef)

	vmSpec := vmDef.Spec.GetYandexCloudVm()
	require.NotNil(t, vmSpec)

	// Check that user-data metadata exists
	require.Contains(t, vmSpec.Metadata, constUserDataKey)
	userDataStr := vmSpec.Metadata[constUserDataKey]
	require.NotEmpty(t, userDataStr)

	// Verify user-data contains cloud-init structure but NO runcmd
	require.Contains(t, userDataStr, "Datasource")
	require.Contains(t, userDataStr, "Users")
	require.Contains(t, userDataStr, "postgres") // username

	// Should NOT contain runcmd or script execution commands for prebuilt images
	require.NotContains(t, userDataStr, "stroppy-cloud-script.sh")
}

func TestCloudBuilder_newVmDef_ScriptingUsesBaseImageId(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	vmRef := &crossplane.Ref{
		Name:      "test-vm",
		Namespace: "test-namespace",
	}
	subnetRef := &crossplane.Ref{
		Name:      "test-subnet",
		Namespace: "test-namespace",
	}
	connectCredsRef := &crossplane.Ref{
		Name:      "test-vm-secret",
		Namespace: "test-namespace",
	}

	baseImageId := "base-ubuntu-2004"
	vm := &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: "10.0.0.70"},
		PublicIp:   true,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  2,
			Memory: 4,
		},
		Strategy: &crossplane.Deployment_Strategy{
			BaseImageId: baseImageId,
			Strategy: &crossplane.Deployment_Strategy_Scripting_{
				Scripting: &crossplane.Deployment_Strategy_Scripting{
					Cmd:     "echo 'test'",
					Workdir: "/tmp",
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name: "user",
		},
	}
	assignIpAddr := &crossplane.Ip{Value: "10.0.0.70"}

	vmDef, err := builder.newVmDef(vmRef, subnetRef, connectCredsRef, vm, assignIpAddr)

	require.NoError(t, err)
	require.NotNil(t, vmDef)

	vmSpec := vmDef.Spec.GetYandexCloudVm()
	require.NotNil(t, vmSpec)
	require.Len(t, vmSpec.BootDisk, 1)
	require.Len(t, vmSpec.BootDisk[0].InitializeParams, 1)

	// For scripting strategy, should use BaseImageId
	require.Equal(t, baseImageId, vmSpec.BootDisk[0].InitializeParams[0].ImageId)
}

func TestCloudBuilder_newVmDef_PrebuiltImageUsesImageId(t *testing.T) {
	config := getTestProviderConfig()
	builder := NewCloudBuilder(config)

	vmRef := &crossplane.Ref{
		Name:      "test-vm",
		Namespace: "test-namespace",
	}
	subnetRef := &crossplane.Ref{
		Name:      "test-subnet",
		Namespace: "test-namespace",
	}
	connectCredsRef := &crossplane.Ref{
		Name:      "test-vm-secret",
		Namespace: "test-namespace",
	}

	prebuiltImageId := "prebuilt-app-image"
	vm := &crossplane.Deployment_Vm{
		InternalIp: &crossplane.Ip{Value: "10.0.0.80"},
		PublicIp:   false,
		MachineInfo: &crossplane.MachineInfo{
			Cores:  2,
			Memory: 4,
		},
		Strategy: &crossplane.Deployment_Strategy{
			BaseImageId: "should-not-be-used",
			Strategy: &crossplane.Deployment_Strategy_PrebuiltImage_{
				PrebuiltImage: &crossplane.Deployment_Strategy_PrebuiltImage{
					ImageId: prebuiltImageId,
				},
			},
		},
		SshUser: &crossplane.SshUser{
			Name: "user",
		},
	}
	assignIpAddr := &crossplane.Ip{Value: "10.0.0.80"}

	vmDef, err := builder.newVmDef(vmRef, subnetRef, connectCredsRef, vm, assignIpAddr)

	require.NoError(t, err)
	require.NotNil(t, vmDef)

	vmSpec := vmDef.Spec.GetYandexCloudVm()
	require.NotNil(t, vmSpec)
	require.Len(t, vmSpec.BootDisk, 1)
	require.Len(t, vmSpec.BootDisk[0].InitializeParams, 1)

	// For prebuilt image strategy, should use PrebuiltImage.ImageId
	require.Equal(t, prebuiltImageId, vmSpec.BootDisk[0].InitializeParams[0].ImageId)
}
