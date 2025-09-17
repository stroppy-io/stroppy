package driver

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

func TestNewDriverClient(t *testing.T) {
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

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
	)
	require.NoError(t, err)
	defer conn.Close()

	protoClient := stroppy.NewDriverPluginClient(conn)
	require.NotNil(t, protoClient)

	client := newDriverClient(protoClient)
	require.NotNil(t, client)
	require.Equal(t, protoClient, client.protoClient)
	require.NotNil(t, client.lg)
}

func TestClient_Initialize(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()

	server := grpc.NewServer()
	testPlugin := &TestPlugin{
		initializeErr: nil,
	}
	stroppy.RegisterDriverPluginServer(server, newDriverServer(testPlugin))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
	)
	require.NoError(t, err)
	defer conn.Close()

	protoClient := stroppy.NewDriverPluginClient(conn)
	client := newDriverClient(protoClient)

	ctx := context.Background()
	stepContext := &stroppy.StepContext{}

	err = client.Initialize(ctx, stepContext)
	require.NoError(t, err)
}

func TestClient_BuildTransactionsFromUnit(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()

	server := grpc.NewServer()
	expectedTransactions := &stroppy.DriverTransactionList{
		Transactions: []*stroppy.DriverTransaction{
			{
				Queries: []*stroppy.DriverQuery{
					{Name: "query1", Request: "SELECT 1"},
				},
			},
		},
	}
	testPlugin := &TestPlugin{
		buildTransactionsResult: expectedTransactions,
		buildTransactionsErr:    nil,
	}
	stroppy.RegisterDriverPluginServer(server, newDriverServer(testPlugin))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
	)
	require.NoError(t, err)
	defer conn.Close()

	protoClient := stroppy.NewDriverPluginClient(conn)
	client := newDriverClient(protoClient)

	ctx := context.Background()
	buildContext := &stroppy.UnitBuildContext{}

	result, err := client.BuildTransactionsFromUnit(ctx, buildContext)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Transactions, 1)
	require.Equal(t, "query1", result.Transactions[0].Queries[0].Name)
}

func TestClient_RunTransaction(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()

	server := grpc.NewServer()
	testPlugin := &TestPlugin{
		runTransactionErr: nil,
	}
	stroppy.RegisterDriverPluginServer(server, newDriverServer(testPlugin))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
	)
	require.NoError(t, err)
	defer conn.Close()

	protoClient := stroppy.NewDriverPluginClient(conn)
	client := newDriverClient(protoClient)

	ctx := context.Background()
	transaction := &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{
			{Name: "test", Request: "SELECT 1"},
		},
	}

	err = client.RunTransaction(ctx, transaction)
	require.NoError(t, err)
}

func TestClient_Teardown(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()

	server := grpc.NewServer()
	testPlugin := &TestPlugin{
		teardownErr: nil,
	}
	stroppy.RegisterDriverPluginServer(server, newDriverServer(testPlugin))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
	)
	require.NoError(t, err)
	defer conn.Close()

	protoClient := stroppy.NewDriverPluginClient(conn)
	client := newDriverClient(protoClient)

	ctx := context.Background()

	err = client.Teardown(ctx)
	require.NoError(t, err)
}

func TestConnectToPlugin(t *testing.T) {
	runConfig := &stroppy.RunConfig{
		Logger: &stroppy.LoggerConfig{
			LogLevel: stroppy.LoggerConfig_LOG_LEVEL_INFO,
			LogMode:  stroppy.LoggerConfig_LOG_MODE_DEVELOPMENT,
		},
		Driver: &stroppy.DriverConfig{
			DriverPluginPath: "test-plugin",
			DriverPluginArgs: []string{"--test"},
		},
	}

	require.NotNil(t, runConfig)
	require.NotNil(t, runConfig.GetLogger())
	require.NotNil(t, runConfig.GetDriver())
	require.Equal(t, "test-plugin", runConfig.GetDriver().GetDriverPluginPath())
	require.Equal(t, []string{"--test"}, runConfig.GetDriver().GetDriverPluginArgs())
}
