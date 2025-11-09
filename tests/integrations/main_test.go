package integrations

import (
	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel/panelconnect"
	"net/http"
	"testing"
)

func newAccountClient() panelconnect.AccountServiceClient {
	return panelconnect.NewAccountServiceClient(http.DefaultClient, "http://localhost:8080")
}

func newAutomateClient() panelconnect.AutomateServiceClient {
	return panelconnect.NewAutomateServiceClient(http.DefaultClient, "http://localhost:8080")
}

const (
	testEmail          = "test@example.com"
	testPassword       = "Password123@"
	defaultPostgresUrl = "postgres://postgres:developer@localhost:5432/postgres"
)

func Test_Main(t *testing.T) {
	accountClient := newAccountClient()
	_, err := accountClient.Register(t.Context(), &panel.RegisterRequest{
		Email:    testEmail,
		Password: testPassword,
	})
	if err != nil {
		if connect.CodeOf(err) == connect.CodeAlreadyExists {
			t.Log("User already exists, skipping registration")
		} else {
			require.NoError(t, err)
		}
	}

	resp, err := accountClient.Login(t.Context(), &panel.LoginRequest{
		Email:    testEmail,
		Password: testPassword,
	})
	require.NoError(t, err)
	t.Log("Login successful")

	ctx, callInfo := connect.NewClientContext(t.Context())
	callInfo.RequestHeader().Set("Authorization", "Bearer "+resp.AccessToken)

	automateClient := newAutomateClient()

	runRecord, err := automateClient.RunAutomation(ctx, &panel.RunAutomationRequest{
		UsingCloudProvider: crossplane.SupportedCloud_SUPPORTED_CLOUD_YANDEX,
		Database: &panel.Database{
			Name:         "test_db",
			DatabaseType: panel.Database_TYPE_POSTGRES_ORIOLE,
			RunnerCluster: &panel.Cluster{
				IsSingleMachineMode: true,
				Machines: []*panel.MachineInfo{
					{
						Cores:  4,
						Memory: 8,
						Disk:   64,
					},
				},
			},
		},
		Workload: &panel.Workload{
			Name:         "test_workload",
			WorkloadType: panel.Workload_TYPE_TPCC,
			RunnerCluster: &panel.Cluster{
				IsSingleMachineMode: true,
				Machines: []*panel.MachineInfo{
					{
						Cores:  2,
						Memory: 4,
						Disk:   64,
					},
				},
			},
		},
	})
	require.NoError(t, err)
	t.Log(runRecord)
}
