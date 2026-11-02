package profiling

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"time"
)

const defaultCPUDuration = 30 * time.Second

var indexTemplate = template.Must(template.New("profiles").Parse(`<!doctype html>
<title>Go profiles</title><h1>Go profiles</h1><ul>
{{range .}}<li><a href="{{.Name}}">{{.Name}}</a></li>{{end}}
<li><a href="profile">CPU profile</a></li><li><a href="trace">Execution trace</a></li>
<li><a href="cmdline">Command line</a></li><li><a href="symbol">Symbols</a></li></ul>`))

// runtime/pprof avoids net/http/pprof's unconditional registration on the host's default mux.
func profileMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/pprof/", serveProfile)
	mux.HandleFunc("GET /debug/pprof/cmdline", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.WriteString(w, strings.Join(os.Args, "\x00")) //nolint:gosec // Plain-text output disables MIME sniffing.
	})
	mux.HandleFunc("GET /debug/pprof/profile", serveCPU)
	mux.HandleFunc("GET /debug/pprof/trace", serveTrace)

	for _, path := range []string{"profile", "trace"} {
		mux.HandleFunc("HEAD /debug/pprof/"+path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
		})
	}

	mux.HandleFunc("GET /debug/pprof/symbol", serveSymbols)

	return mux
}

func serveProfile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/debug/pprof/")
	if name == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = indexTemplate.Execute(w, pprof.Profiles())

		return
	}

	profile := pprof.Lookup(name)
	if profile == nil {
		http.NotFound(w, r)

		return
	}

	if name == "heap" && r.URL.Query().Get("gc") == "1" {
		runtime.GC()
	}

	debug, _ := strconv.Atoi(r.URL.Query().Get("debug"))
	if debug < 0 {
		debug = 0
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/octet-stream")

	if debug > 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	_ = profile.WriteTo(w, debug)
}

func captureDuration(r *http.Request, fallback time.Duration) (time.Duration, error) {
	value := r.URL.Query().Get("seconds")
	if value == "" {
		return fallback, nil
	}

	// ParseDuration rejects overflow, NaN, and infinity before converting to nanoseconds.
	return time.ParseDuration(value + "s")
}

func waitCapture(r *http.Request, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-r.Context().Done():
	}
}

func serveCPU(w http.ResponseWriter, r *http.Request) {
	duration, err := captureDuration(r, defaultCPUDuration)
	if err != nil || duration <= 0 {
		http.Error(w, "seconds must be a positive duration in seconds", http.StatusBadRequest)

		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")

	if err := pprof.StartCPUProfile(w); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)

		return
	}

	defer pprof.StopCPUProfile()

	waitCapture(r, duration)
}

func serveTrace(w http.ResponseWriter, r *http.Request) {
	duration, err := captureDuration(r, time.Second)
	if err != nil || duration <= 0 {
		http.Error(w, "seconds must be a positive duration in seconds", http.StatusBadRequest)

		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")

	if err := trace.Start(w); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)

		return
	}

	defer trace.Stop()

	waitCapture(r, duration)
}

func serveSymbols(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "num_symbols: 1")

	for value := range strings.SplitSeq(r.URL.RawQuery, "+") {
		address, err := strconv.ParseUint(value, 0, strconv.IntSize)
		if err != nil {
			continue
		}

		if function := runtime.FuncForPC(uintptr(address)); function != nil {
			fmt.Fprintf(w, "%s %s\n", value, function.Name())
		}
	}
}
