package driver

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/core/utils/errchan"
)

func TestPluginHandshake(t *testing.T) {
	require.Equal(t, uint(pluginVersion), PluginHandshake.ProtocolVersion)
	require.Equal(t, magicCookieKey, PluginHandshake.MagicCookieKey)
	require.Equal(t, magicCookieValue, PluginHandshake.MagicCookieValue)
}

func TestNewSharedPlugin(t *testing.T) {
	testPlugin := &TestPlugin{}
	sharedPlugin := NewSharedPlugin(testPlugin)

	require.NotNil(t, sharedPlugin)
	require.Equal(t, testPlugin, sharedPlugin.Impl)
}

func TestSharedPlugin_GRPCServer(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()

	server := grpc.NewServer()
	testPlugin := &TestPlugin{}
	sharedPlugin := NewSharedPlugin(testPlugin)

	err := sharedPlugin.GRPCServer(nil, server)
	require.NoError(t, err)

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

	client := stroppy.NewDriverPluginClient(conn)
	require.NotNil(t, client)

	ctx := context.Background()
	stepContext := &stroppy.StepContext{}

	_, err = client.Initialize(ctx, stepContext)
}

func TestSharedPlugin_GRPCClient(t *testing.T) {
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

	sharedPlugin := NewSharedPlugin(nil)

	client, err := sharedPlugin.GRPCClient(context.Background(), nil, conn)
	require.NoError(t, err)
	require.NotNil(t, client)

	pluginClient, ok := client.(Plugin)
	require.True(t, ok)
	require.NotNil(t, pluginClient)
}

func TestPluginInterface(t *testing.T) {
	var _ Plugin = (*TestPlugin)(nil)

	testPlugin := &TestPlugin{}

	ctx := context.Background()
	stepContext := &stroppy.StepContext{}
	buildContext := &stroppy.UnitBuildContext{}
	transaction := &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{
			{Name: "test", Request: "SELECT 1"},
		},
	}

	err := testPlugin.Initialize(ctx, stepContext)
	require.NoError(t, err)

	result, err := testPlugin.BuildTransactionsFromUnit(ctx, buildContext)
	require.NoError(t, err)
	require.Nil(t, result)

	channel, err := testPlugin.BuildTransactionsFromUnitStream(ctx, buildContext)
	require.NoError(t, err)
	require.Nil(t, channel)

	err = testPlugin.RunTransaction(ctx, transaction)
	require.NoError(t, err)

	err = testPlugin.Teardown(ctx)
	require.NoError(t, err)
}

func TestPluginWithErrors(t *testing.T) {
	testPlugin := &TestPlugin{
		initializeErr:              errors.New("init error"),
		buildTransactionsErr:       errors.New("build error"),
		buildTransactionsStreamErr: errors.New("stream error"),
		runTransactionErr:          errors.New("run error"),
		teardownErr:                errors.New("teardown error"),
	}

	ctx := context.Background()
	stepContext := &stroppy.StepContext{}
	buildContext := &stroppy.UnitBuildContext{}
	transaction := &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{
			{Name: "test", Request: "SELECT 1"},
		},
	}

	err := testPlugin.Initialize(ctx, stepContext)
	require.Error(t, err)
	require.Equal(t, "init error", err.Error())

	result, err := testPlugin.BuildTransactionsFromUnit(ctx, buildContext)
	require.Error(t, err)
	require.Equal(t, "build error", err.Error())
	require.Nil(t, result)

	channel, err := testPlugin.BuildTransactionsFromUnitStream(ctx, buildContext)
	require.Error(t, err)
	require.Equal(t, "stream error", err.Error())
	require.Nil(t, channel)

	err = testPlugin.RunTransaction(ctx, transaction)
	require.Error(t, err)
	require.Equal(t, "run error", err.Error())

	err = testPlugin.Teardown(ctx)
	require.Error(t, err)
	require.Equal(t, "teardown error", err.Error())
}

func TestPluginWithResults(t *testing.T) {
	expectedTransactions := &stroppy.DriverTransactionList{
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
	}

	streamChannel := make(errchan.Chan[stroppy.DriverTransaction])
	go func() {
		errchan.Send(streamChannel, &stroppy.DriverTransaction{
			Queries: []*stroppy.DriverQuery{{Name: "stream1"}},
		}, nil)
		errchan.Send(streamChannel, &stroppy.DriverTransaction{
			Queries: []*stroppy.DriverQuery{{Name: "stream2"}},
		}, nil)
		errchan.Close[stroppy.DriverTransaction](streamChannel)
	}()

	testPlugin := &TestPlugin{
		buildTransactionsResult:        expectedTransactions,
		buildTransactionsStreamChannel: streamChannel,
	}

	ctx := context.Background()
	buildContext := &stroppy.UnitBuildContext{}

	result, err := testPlugin.BuildTransactionsFromUnit(ctx, buildContext)
	require.NoError(t, err)
	require.Equal(t, expectedTransactions, result)
	require.Len(t, result.Transactions, 2)
	require.Equal(t, "query1", result.Transactions[0].Queries[0].Name)
	require.Equal(t, "query2", result.Transactions[1].Queries[0].Name)

	channel, err := testPlugin.BuildTransactionsFromUnitStream(ctx, buildContext)
	require.NoError(t, err)
	require.NotNil(t, channel)

	tx1, err := errchan.Receive[stroppy.DriverTransaction](channel)
	require.NoError(t, err)
	require.Equal(t, "stream1", tx1.Queries[0].Name)

	tx2, err := errchan.Receive[stroppy.DriverTransaction](channel)
	require.NoError(t, err)
	require.Equal(t, "stream2", tx2.Queries[0].Name)
}
