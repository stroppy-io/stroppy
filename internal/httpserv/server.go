package httpserv

import (
	connectcors "connectrpc.com/cors"
	"github.com/rs/cors"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/api"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/claims"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/probes"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel/panelconnect"
)

type HTTPServerConfig struct {
	Host         string        `mapstructure:"host" default:"0.0.0.0" validate:"required,hostname|ip"`
	Port         int           `mapstructure:"port" default:"8080" validate:"required,min=1,max=65535"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" default:"5s"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" default:"10s"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout" default:"120s"`
	CorsDomain   string        `mapstructure:"cors_domain" default:"*"`
	StaticDir    string        `mapstructure:"static_dir" default:""`
}

func addStarToPath(path string, h http.Handler) (string, http.Handler) {
	return path + "*", h
}

func NewServer(
	config *HTTPServerConfig,
	log *zap.Logger,
	startupProbe probes.Probe,
	readyProbe probes.Probe,
	userTokenProtector TokenProtector,
	service *api.PanelService,
) (*http.Server, error) {
	connectProtocols := new(http.Protocols)
	connectProtocols.SetHTTP1(true)
	connectProtocols.SetHTTP2(true)
	connectProtocols.SetUnencryptedHTTP2(true)
	mux := chi.NewMux()
	mux.Use(cors.New(cors.Options{
		//AllowedOrigins: []string{config.CorsDomain}, // TODO: replace with your domain
		AllowedMethods: connectcors.AllowedMethods(),
		AllowedHeaders: append(connectcors.AllowedHeaders(), "Authorization"),
		ExposedHeaders: connectcors.ExposedHeaders(),
		MaxAge:         7200, // 2 hours in seconds
		Debug:          log.Level() == zap.DebugLevel,
		Logger:         newCorsLogger(log.Named("connect-cors")),
	}).Handler)
	mux.Handle(probes.DefaultLivenessProbePath, probes.NewLivenessProbe(readyProbe))
	mux.Handle(probes.DefaultReadinessProbePath, probes.NewReadinessProbe(readyProbe))
	mux.Handle(probes.DefaultStartupProbePath, probes.NewStartupProbe(startupProbe))
	intercept := connect.WithInterceptors(
		loggerMiddleware(log.Named("connect")),
		authMiddleware[claims.Claims](
			protectedGrpcService(
				userTokenProtector,
				[]grpc.ServiceDesc{
					panel.AccountService_ServiceDesc,
					panel.ResourcesService_ServiceDesc,
					panel.RunService_ServiceDesc,
					panel.AutomateService_ServiceDesc,
				},
				WithExcluding(panel.AccountService_Login_FullMethodName),
				WithExcluding(panel.AccountService_Register_FullMethodName),
				WithExcluding(panel.AccountService_RefreshTokens_FullMethodName),
				WithExcluding(panel.RunService_ListTopRuns_FullMethodName),
			),
		),
		recoveryMiddleware(NoRecoveryHandlerFuncContext),
	)
	mux.Handle(addStarToPath(panelconnect.NewAccountServiceHandler(service, intercept)))
	mux.Handle(addStarToPath(panelconnect.NewAutomateServiceHandler(service, intercept)))
	mux.Handle(addStarToPath(panelconnect.NewResourcesServiceHandler(service, intercept)))
	mux.Handle(addStarToPath(panelconnect.NewRunServiceHandler(service, intercept)))
	if err := registerStaticFrontend(mux, config.StaticDir, log.Named("static")); err != nil {
		log.Warn("static file handler disabled", zap.String("path", config.StaticDir), zap.Error(err))
	}
	httpServer := &http.Server{
		Addr:         net.JoinHostPort(config.Host, strconv.Itoa(config.Port)),
		Handler:      mux,
		ErrorLog:     zap.NewStdLog(log.Named("http")),
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
		Protocols:    connectProtocols,
	}
	return httpServer, nil
}
