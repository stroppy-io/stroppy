package httpserv

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func registerStaticFrontend(mux *chi.Mux, staticDir string, log *zap.Logger) error {
	if staticDir == "" {
		return nil
	}

	handler, err := newSPAHandler(staticDir)
	if err != nil {
		return fmt.Errorf("failed to initialize static handler: %w", err)
	}

	mux.NotFound(handler)
	log.Info("static frontend handler enabled", zap.String("directory", staticDir))
	return nil
}

func newSPAHandler(staticDir string) (http.HandlerFunc, error) {
	info, err := os.Stat(staticDir)
	if err != nil {
		return nil, fmt.Errorf("static directory unavailable: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("static path is not a directory: %s", staticDir)
	}

	indexPath := filepath.Join(staticDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return nil, fmt.Errorf("index file missing: %w", err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		cleaned := path.Clean("/" + r.URL.Path)
		if cleaned == "/" {
			http.ServeFile(w, r, indexPath)
			return
		}

		relativePath := strings.TrimPrefix(cleaned, "/")
		fullPath := filepath.Join(staticDir, relativePath)

		if fileInfo, err := os.Stat(fullPath); err == nil && !fileInfo.IsDir() {
			http.ServeFile(w, r, fullPath)
			return
		}

		http.ServeFile(w, r, indexPath)
	}, nil
}
