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

	provider := resource.NewYandexCloudProvider(&resource.YandexCloudProviderConfig{
		DefaultNetworkId:        "enp7b429s2br5pja0jci",
		DefaultSubnetId:         "fl85dq0cam3ed6cfg2st",
		DefaultNetworkCidrBlock: "10.1.0.0/16",
		DefaultVmZone:           "ru-central1-d",
		DefaultVmPlatformId:     "standard-v2",
		DefaultUbuntuImageId:    "fd82pkek8uu0ejjkh4vn",
	})
	dataBuilder := resource.NewBuilder(provider)
	api, err := NewCrossplaneApi("./kubeconfig.yaml", dataBuilder)
	if err != nil {
		t.Errorf("failed to create crossplane api: %v", err)
	}
	automationId := ids.NewUlid()
	res, err := dataBuilder.NewDatabaseResources(automationId, &panel.Database{
		Name:         "some_postgres",
		DatabaseType: panel.Database_TYPE_POSTGRES_ORIOLE,
		Parameters: map[string]string{
			"username": "postgres",
			"password": "postgres",
		},
		RunnerCluster: &panel.Cluster{
			IsSingleMachineMode: true,
			Machines: []*panel.MachineInfo{
				{
					Cores:  2,
					Memory: 4,
					Disk:   10,
				},
			},
		},
	},
		&panel.Script{},
	)

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
