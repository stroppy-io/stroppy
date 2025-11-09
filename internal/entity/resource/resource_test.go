package resource

import (
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"testing"
)

func TestNewDatabaseResources(t *testing.T) {
	provider := NewYandexCloudBuilder(&YandexCloudProviderConfig{
		DefaultNetworkId:        "enp7b429s2br5pja0jci",
		DefaultSubnetId:         "fl85dq0cam3ed6cfg2st",
		DefaultNetworkCidrBlock: "10.1.0.0/16",
		DefaultVmZone:           "ru-central1-d",
		DefaultVmPlatformId:     "standard-v2",
		DefaultUbuntuImageId:    "fd82pkek8uu0ejjkh4vn",
	})

	database := &panel.Database{
		Name: "some_postgres",
		RunnerCluster: &panel.Cluster{
			Machines: []*panel.MachineInfo{
				{
					Cores:  4,
					Memory: 8,
					Disk:   64,
				},
			},
		},
	}
	resource, err := provider.NewSingleVmResource("test-vm", database.RunnerCluster.Machines[0], &panel.Script{})
	if err != nil {
		t.Errorf("failed to create database resources: %v", err)
	}

	t.Log(resource)
	net := resource
	t.Log(net.Resource.Resource.GetResourceYaml())
	subnet := net.Children[0]
	t.Log(subnet.Resource.Resource.GetResourceYaml())
	vm := subnet.Children[0]
	t.Log(vm.Resource.Resource.GetResourceYaml())
}
