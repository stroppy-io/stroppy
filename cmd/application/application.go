package application

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/api"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/api/repositories"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/api/tasks"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/build"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/configurator"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/logger"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/probes"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/shutdown"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/token"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/deployment"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/provider/yandex"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/workflow"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/httpserv"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/crossplaneservice"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/crossplaneservice/k8s"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

const (
	panelServiceNameLoggerName = "panel-service"
	constantTaskLoggerName     = "task-processor"
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
	validateCfgErr := cfg.AdditionalValidate()
	if validateCfgErr != nil {
		return nil, fmt.Errorf("failed to validate config: %w", validateCfgErr)
	}
	appLogger := logger.NewFromConfig(&cfg.Service.Logger)
	appLogger.Info(
		"Application initialized",
		zap.String("service", build.ServiceName),
		zap.String("version", build.Version),
	)
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

	k8sActor, err := k8s.NewClient(cfg.Service.K8S.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create K8S client: %w", err)
	}
	crossplaneClient := crossplaneservice.NewCrossplaneService(k8sActor, cfg.Service.Crossplane.ReconcileInterval)

	runRecordRepo := repositories.NewRunRecordRepository(executor)
	quotaRepository := repositories.NewQuotaRepository(executor)
	workflowRepo := repositories.NewWorkflowRepository(executor)

	yandexBuilder := yandex.NewCloudBuilder(&cfg.Service.Cloud.Yandex.Provider)
	deploymentBuilder := deployment.NewBuilder(
		map[crossplane.SupportedCloud]deployment.DagBuilder{
			crossplane.SupportedCloud_SUPPORTED_CLOUD_YANDEX: yandexBuilder,
		},
	)

	service := api.NewPanelService(
		appLogger.Named(panelServiceNameLoggerName),
		executor,
		txManager,
		tokenActor,
		&cfg.Service.Workflow,
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

	taskProcessor, err := workflow.NewTaskProcessor(
		&cfg.Service.Workflow,
		appLogger.Named(constantTaskLoggerName),
		txManager,
		workflowRepo,
		map[panel.WorkflowTask_Type]workflow.TaskWrapperBuilder{
			panel.WorkflowTask_TYPE_DEPLOY_DATABASE: workflow.NewTaskBuilder(
				tasks.NewDeployDatabaseTaskHandler(quotaRepository, crossplaneClient, deploymentBuilder),
			),
			panel.WorkflowTask_TYPE_DEPLOY_STROPPY: workflow.NewTaskBuilder(
				tasks.NewDeployStroppyTaskHandler(quotaRepository, yandexBuilder, crossplaneClient, deploymentBuilder),
			),
			panel.WorkflowTask_TYPE_COLLECT_RUN_RESULTS: workflow.NewTaskBuilder(
				tasks.NewCollectRunResultTaskHandler(runRecordRepo),
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create task processor: %w", err)
	}
	shutdown.RegisterFn(taskProcessor.Start())

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
