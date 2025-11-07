package resource

import (
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"testing"
)

func TestNewDatabaseResources(t *testing.T) {
	provider := NewYandexCloudProvider(&YandexCloudProviderConfig{
		DefaultNetworkId:        "enp7b429s2br5pja0jci",
		DefaultSubnetId:         "fl85dq0cam3ed6cfg2st",
		DefaultNetworkCidrBlock: "10.1.0.0/16",
		DefaultVmZone:           "ru-central1-d",
		DefaultVmPlatformId:     "standard-v2",
		DefaultUbuntuImageId:    "fd82pkek8uu0ejjkh4vn",
	})
	dataBuilder := NewBuilder(provider)

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
	automationId := ids.NewUlid()

	resource, err := dataBuilder.NewDatabaseResources(automationId, database, &panel.Script{})
	if err != nil {
		t.Errorf("failed to create database resources: %v", err)
	}

	t.Log(resource)
	t.Log(resource.Children[0])
	vm := resource.Children[0].Children[0]
	t.Log(vm.Resource.Resource.GetResourceYaml())
}
