package probes

import (
	"context"
	"net/http"
	"time"
)

// WaitProbe waits for a probe to complete within a specified timeout.
//
// ctx is the parent context for the probe, probe is the probe to wait for, and timeout is the maximum time to wait.
// ProbeStatus indicating the result of the probe.
func WaitProbe(ctx context.Context, probe Probe, timeout time.Duration) ProbeStatus {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resCh := make(chan ProbeStatus, 1)
	go func(ctx context.Context) {
		select {
		case <-ctx.Done():
			resCh <- TimeoutProbe
		default:
			resCh <- probe.HealthCheck(waitCtx)
		}
	}(waitCtx)

	return <-resCh
}

const DefaultProbeTimeout = 30 * time.Second

// NewProbeHandler returns an http.HandlerFunc that waits for the probe to complete
// within DefaultProbeTimeout, and returns a response with a 200 status code if
// the probe succeeds, or a 503 status code if the probe fails or times out.
// The response body will be "Healthy" or "Unhealthy <name>" respectively.
func NewProbeHandler(name string, probe Probe) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		probeStatus := WaitProbe(r.Context(), probe, DefaultProbeTimeout)

		if probeStatus == SuccessProbe {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Healthy"))

			return
		}

		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("Unhealthy " + name))
	}
}

type HTTPProbe struct {
	path string
	http.Handler
}

func (p *HTTPProbe) Path() string {
	return p.path
}

func (p *HTTPProbe) Method() string {
	return http.MethodGet
}

// NewHTTPProbe returns a new HTTPProbe with the given path and handler.
//
// The path will be used as the ConnStr path for the probe, and the handler will be
// used to handle requests to that path.
//
// The handler will be wrapped with a timeout of DefaultProbeTimeout.
func NewHTTPProbe(path string, handler http.Handler) *HTTPProbe {
	return &HTTPProbe{path: path, Handler: handler}
}

const (
	DefaultLivenessProbePath  = "/healthz/liveness"
	DefaultReadinessProbePath = "/healthz/readiness"
	DefaultStartupProbePath   = "/healthz/startup"
)

// NewLivenessProbe returns an HTTPProbe for liveness checks.
//
// probe is the Probe instance to be used for the liveness check.
// Returns an *HTTPProbe instance.
func NewLivenessProbe(probe Probe) *HTTPProbe {
	return NewHTTPProbe(DefaultLivenessProbePath, NewProbeHandler("Liveness", probe))
}

// NewReadinessProbe returns an HTTPProbe for readiness checks.
//
// probe is the Probe instance to be used for the readiness check.
// Returns an *HTTPProbe instance.
func NewReadinessProbe(probe Probe) *HTTPProbe {
	return NewHTTPProbe(DefaultReadinessProbePath, NewProbeHandler("Readiness", probe))
}

// NewStartupProbe returns an HTTPProbe for startup checks.
//
// probe is the Probe instance to be used for the startup check.
// Returns an *HTTPProbe instance.
func NewStartupProbe(probe Probe) *HTTPProbe {
	return NewHTTPProbe(DefaultStartupProbePath, NewProbeHandler("Startup", probe))
}

type ServeConfig struct {
	Host string `mapstructure:"host" default:"0.0.0.0" validate:"required,ip"`
	Port int    `mapstructure:"port" default:"8080" validate:"required,min=1,max=65535"`
}

// NewServingMux creates a new http.ServeMux and registers all probes from the input list with it.
//
// The returned *http.ServeMux is ready to be used with an http.Server.
//
// Example:
//
//	probes := []*probes.HTTPProbe{
//		probes.NewLivenessProbe(probe.NewBoolProbe()),
//		probes.NewReadinessProbe(probe.NewBoolProbe()),
//	}
//
//	server := &http.Server{
//		Addr:    ":8080",
//		Handler: probes.NewServingMux(probes...),
//	}
//
//	server.ListenAndServe()
func NewServingMux(probes ...*HTTPProbe) *http.ServeMux {
	mux := http.NewServeMux()

	for _, probe := range probes {
		mux.Handle(probe.path, probe.Handler)
	}

	return mux
}
