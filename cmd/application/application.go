package application

import (
	"fmt"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/api"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/httpserv"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"net/http"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/build"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/configurator"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/probes"
	"github.com/stroppy-io/stroppy/pkg/core/logger"
	"github.com/stroppy-io/stroppy/pkg/core/shutdown"

	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
)

type Application struct {
	config *Config

	server *http.Server
}

const (
	restLoggerName  = "komeet-server-rest"
	restTracingName = "komeet-server-rest"
	s3LoggerName    = "komeet-server-s3"
	rpcLoggerName   = "komeet-server-rpc"
	amqpLoggerName  = "komeet-server-amqp"
)

var (
	startupProbe = probes.NewBoolProbe()
)

func New() (*Application, error) {
	cfg, err := configurator.LoadConfig[Config]()
	if err != nil {
		return nil, err
	}
	appLogger := logger.NewFromConfig(&cfg.Service.Logger)
	appLogger.Info(
		"Application initialized",
		zap.String("service", build.ServiceName),
		zap.String("version", build.Version),
	)
	//s3Client := s3.NewS3Client(&cfg.Infra.S3, logger.NewStructLogger(s3LoggerName))
	//
	pgxPool, err := NewDatabasePollWithTx(cfg)
	if err != nil {
		return nil, err
	}
	executor, txManager, err := postgres.NewTxFlow(pgxPool, postgres.SerializableSettings())
	if err != nil {
		return nil, fmt.Errorf("failed to create TxFlow: %w", err)
	}
	tokenActor := token.NewTokenActor(&cfg.Service.Auth)
	readyProbe := NewReadyProbe(pgxPool)

	grpcHandler := api.NewPanelService(executor, txManager)
	server, err := httpserv.NewServer(
		&cfg.Service.Server,
		appLogger,
		startupProbe,
		readyProbe,
		tokenActor,
		grpcHandler,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create http server: %w", err)
	}
	return &Application{
		config: cfg,
		server: server,
	}, nil
}

func (a *Application) Run() error {
	shutdown.RegisterFn(func() {
		//err := a.rpcServer.Close()
		//if err != nil {
		//logger.Error("failed to close rpc server", zap.Error(err))
		//}
		logger.Info(
			"Application stopped",
			zap.String("service", build.ServiceName),
			zap.String("version", build.Version),
		)
	})
	defer func() {
		startupProbe.Set(true)
		logger.Info(
			"Application started",
			zap.String("service", build.ServiceName),
			zap.String("version", build.Version),
		)
	}()
	go func() {
		logger.Info(
			"REST server started",
			zap.String("host", a.config.Service.REST.Host),
			zap.Int("port", a.config.Service.REST.Port),
		)
		err := a.restServer.Serve()
		if err != nil {
			logger.Error("failed to start rest server", zap.Error(err))
		}
	}()
	return grpc.RunGrpcServer(&a.config.Service.GRPC, a.server)
}
