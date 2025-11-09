package automate

import (
	"github.com/stretchr/testify/require"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/resource"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"testing"
	"time"
)

func TestCrossplaneApi_CreateResource(t *testing.T) {
	config := resource.YandexCloudProviderConfig{
		DefaultNetworkId:        "enp7b429s2br5pja0jci",
		DefaultSubnetId:         "fl85dq0cam3ed6cfg2st",
		DefaultNetworkCidrBlock: "10.1.0.0/16",
		DefaultVmZone:           "ru-central1-d",
		DefaultVmPlatformId:     "standard-v2",
		DefaultUbuntuImageId:    "fd82pkek8uu0ejjkh4vn",
	}
	provider := resource.NewYandexCloudBuilder(&config)
	api, err := NewCrossplaneApi("./kubeconfig.yaml")
	if err != nil {
		t.Errorf("failed to create crossplane api: %v", err)
	}
	automationId := ids.NewUlid()
	res, err := provider.NewSingleVmResource(automationId, "test-vm", &panel.MachineInfo{
		Cores:  2,
		Memory: 4,
		Disk:   10,
	}, &panel.Script{})
	if err != nil {
		t.Errorf("failed to create database resources: %v", err)
	}
	t.Log(res)
	t.Log(res.Children[0])
	vm := res.Children[0].Children[0]
	t.Log(vm.Resource.Resource.GetResourceYaml())

	// NOTE: ITS ONLY CREATED NETWORK CAUSE NewDatabaseResources RETURNS ONLY TREE NODES
	require.NoError(t, err)
	ref := res.GetResource().GetResource().GetRef()
	def := res.GetResource().GetResource().GetResourceDef()
	_resp, err := api.CreateResource(t.Context(), &crossplane.CreateResourceRequest{
		Resource: def,
		Ref:      ref,
	})
	require.NoError(t, err)
	t.Log(_resp)
	time.Sleep(10 * time.Second)
	_status, err := api.GetResourceStatus(t.Context(), &crossplane.GetResourceStatusRequest{
		Ref: resource.ExtRefFromResourceDef(ref, def),
	})
	require.NoError(t, err)
	t.Log(_status)
	time.Sleep(10 * time.Second)
	_, err = api.DeleteResource(t.Context(), &crossplane.DeleteResourceRequest{
		Ref: resource.ExtRefFromResourceDef(ref, def),
	})
	require.NoError(t, err)

}
