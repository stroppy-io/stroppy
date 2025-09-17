package driver

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

type TestPlugin struct {
	initializeErr                  error
	buildTransactionsErr           error
	buildTransactionsStreamErr     error
	runTransactionErr              error
	teardownErr                    error
	buildTransactionsResult        *stroppy.DriverTransactionList
	buildTransactionsStreamChannel errchan.Chan[stroppy.DriverTransaction]
}

func (t *TestPlugin) Initialize(
	_ context.Context,
	_ *stroppy.StepContext,
) error {
	return t.initializeErr
}

func (t *TestPlugin) BuildTransactionsFromUnit(
	_ context.Context,
	_ *stroppy.UnitBuildContext,
) (*stroppy.DriverTransactionList, error) {
	return t.buildTransactionsResult, t.buildTransactionsErr
}

func (t *TestPlugin) BuildTransactionsFromUnitStream(
	_ context.Context,
	_ *stroppy.UnitBuildContext,
) (errchan.Chan[stroppy.DriverTransaction], error) {
	return t.buildTransactionsStreamChannel, t.buildTransactionsStreamErr
}

func (t *TestPlugin) RunTransaction(
	_ context.Context,
	_ *stroppy.DriverTransaction,
) error {
	return t.runTransactionErr
}

func (t *TestPlugin) Teardown(_ context.Context) error {
	return t.teardownErr
}

func TestNewDriverServer(t *testing.T) {
	testPlugin := &TestPlugin{}
	server := newDriverServer(testPlugin)

	require.NotNil(t, server)
	require.Equal(t, testPlugin, server.impl)
	require.NotNil(t, server.UnimplementedDriverPluginServer)
}

func TestServer_Initialize(t *testing.T) {
	tests := []struct {
		name    string
		plugin  *TestPlugin
		wantErr bool
	}{
		{
			name: "успешная инициализация",
			plugin: &TestPlugin{
				initializeErr: nil,
			},
			wantErr: false,
		},
		{
			name: "ошибка инициализации",
			plugin: &TestPlugin{
				initializeErr: errors.New("init error"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newDriverServer(tt.plugin)
			ctx := context.Background()
			stepContext := &stroppy.StepContext{}

			result, err := server.Initialize(ctx, stepContext)

			if tt.wantErr {
				require.Error(t, err)
				require.NotNil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.IsType(t, &emptypb.Empty{}, result)
			}
		})
	}
}

func TestServer_BuildTransactionsFromUnit(t *testing.T) {
	tests := []struct {
		name    string
		plugin  *TestPlugin
		want    *stroppy.DriverTransactionList
		wantErr bool
	}{
		{
			name: "успешное создание транзакций",
			plugin: &TestPlugin{
				buildTransactionsResult: &stroppy.DriverTransactionList{
					Transactions: []*stroppy.DriverTransaction{
						{
							Queries: []*stroppy.DriverQuery{
								{Name: "query1", Request: "SELECT 1"},
							},
						},
						{
							Queries: []*stroppy.DriverQuery{
								{Name: "query2", Request: "SELECT 2"},
							},
						},
					},
				},
				buildTransactionsErr: nil,
			},
			want: &stroppy.DriverTransactionList{
				Transactions: []*stroppy.DriverTransaction{
					{
						Queries: []*stroppy.DriverQuery{
							{Name: "query1", Request: "SELECT 1"},
						},
					},
					{
						Queries: []*stroppy.DriverQuery{
							{Name: "query2", Request: "SELECT 2"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "ошибка создания транзакций",
			plugin: &TestPlugin{
				buildTransactionsResult: nil,
				buildTransactionsErr:    errors.New("build error"),
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newDriverServer(tt.plugin)
			ctx := context.Background()
			buildContext := &stroppy.UnitBuildContext{}

			result, err := server.BuildTransactionsFromUnit(ctx, buildContext)

			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, result)
			}
		})
	}
}

func TestServer_RunTransaction(t *testing.T) {
	tests := []struct {
		name    string
		plugin  *TestPlugin
		wantErr bool
	}{
		{
			name: "успешное выполнение транзакции",
			plugin: &TestPlugin{
				runTransactionErr: nil,
			},
			wantErr: false,
		},
		{
			name: "ошибка выполнения транзакции",
			plugin: &TestPlugin{
				runTransactionErr: errors.New("run error"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newDriverServer(tt.plugin)
			ctx := context.Background()
			transaction := &stroppy.DriverTransaction{
				Queries: []*stroppy.DriverQuery{
					{Name: "test", Request: "SELECT 1"},
				},
			}

			result, err := server.RunTransaction(ctx, transaction)

			if tt.wantErr {
				require.Error(t, err)
				require.NotNil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.IsType(t, &emptypb.Empty{}, result)
			}
		})
	}
}

func TestServer_Teardown(t *testing.T) {
	tests := []struct {
		name    string
		plugin  *TestPlugin
		wantErr bool
	}{
		{
			name: "успешное завершение",
			plugin: &TestPlugin{
				teardownErr: nil,
			},
			wantErr: false,
		},
		{
			name: "ошибка завершения",
			plugin: &TestPlugin{
				teardownErr: errors.New("teardown error"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newDriverServer(tt.plugin)
			ctx := context.Background()

			result, err := server.Teardown(ctx, &emptypb.Empty{})

			if tt.wantErr {
				require.Error(t, err)
				require.NotNil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.IsType(t, &emptypb.Empty{}, result)
			}
		})
	}
}

func TestServePlugin(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()

	server := grpc.NewServer()
	testPlugin := &TestPlugin{}
	stroppy.RegisterDriverPluginServer(server, newDriverServer(testPlugin))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	defer server.Stop()

	require.NotNil(t, server)

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := stroppy.NewDriverPluginClient(conn)
	require.NotNil(t, client)

	ctx := context.Background()
	stepContext := &stroppy.StepContext{}

	_, err = client.Initialize(ctx, stepContext)
}

func TestServer_BuildTransactionsFromUnitStream(t *testing.T) {
	testPlugin := &TestPlugin{
		buildTransactionsStreamErr: errors.New("stream error"),
	}
	server := newDriverServer(testPlugin)
	buildContext := &stroppy.UnitBuildContext{}

	require.NotNil(t, server)
	require.Equal(t, testPlugin, server.impl)
	require.NotNil(t, buildContext)
}
