package application

import (
	"fmt"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/crossplaneservice"
	"go.uber.org/zap"
	"net/http"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/api"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/automate"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/build"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/configurator"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/probes"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/shutdown"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/httpserv"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
)

const (
	panelServiceNameLoggerName = "panel-service"
)

type Application struct {
	config *Config

	server *http.Server
}

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

	crossplaneImpl, err := automate.NewCrossplaneApi(cfg.Service.K8S.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create CrossplaneApiImpl: %w", err)
	}
	crossplaneClient, cancel := crossplaneservice.NewLocalCrossplaneClient(crossplaneImpl)
	if cancel != nil {
		shutdown.RegisterFn(cancel)
	}
	service := api.NewPanelService(
		appLogger.Named(panelServiceNameLoggerName),
		executor,
		txManager,
		tokenActor,
		&cfg.Service.K8S,
		&cfg.Service.Automate,
		crossplaneClient,
	)
	server, err := httpserv.NewServer(
		&cfg.Service.Server,
		appLogger,
		startupProbe,
		readyProbe,
		tokenActor,
		service,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create http server: %w", err)
	}
	cancelAutomate, err := automate.NewBackgroundWorker(&cfg.Service.Background, appLogger, service)
	if err != nil {
		return nil, fmt.Errorf("failed to create background worker: %w", err)
	}
	shutdown.RegisterFn(cancelAutomate)

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
			"HTTP server started",
			zap.String("host", a.config.Service.Server.Host),
			zap.Int("port", a.config.Service.Server.Port),
		)
		err := a.server.ListenAndServe()
		if err != nil {
			logger.Error("failed to start rest server", zap.Error(err))
		}
	}()
	return nil
}
