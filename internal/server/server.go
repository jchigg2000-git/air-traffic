// Package server wires the Air-Traffic HTTP API, the synthetic vendor surfaces,
// and the SPA fallback (Phase 2). Stdlib-only, mirrors it-scorecard.
package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"air-traffic/internal/audit"
	"air-traffic/internal/store"
	"air-traffic/internal/synthetic"
)

// Server holds dependencies for the HTTP handlers.
type Server struct {
	store     *store.Store
	log       *slog.Logger
	synthetic *synthetic.Handler
}

// New constructs a Server and seeds the audit stream.
func New(st *store.Store, log *slog.Logger) *Server {
	for _, e := range audit.Seed() {
		st.AddAudit(e)
	}
	return &Server{store: st, log: log, synthetic: synthetic.New(st, log)}
}

// Routes returns the fully wired handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/adapters", s.handleAdapters)
	mux.HandleFunc("/api/adapters/", s.handleAdapter)
	mux.HandleFunc("/api/baselines", s.handleBaselines)
	mux.HandleFunc("/api/policies", s.handlePolicies)
	mux.HandleFunc("/api/credentials", s.handleCredentials)
	mux.HandleFunc("/api/observations", s.handleObservations)
	mux.HandleFunc("/api/audit", s.handleAudit)
	mux.HandleFunc("/api/activity", s.handleActivity)
	mux.HandleFunc("/api/drift", s.handleDrift)
	mux.HandleFunc("/api/cost/facets", s.handleCostFacets)
	mux.HandleFunc("/api/envconfig", s.handleEnvConfig)

	mux.Handle("/synthetic/", s.synthetic)

	// SPA fallback (Phase 2): serve web/dist if present.
	if fileExists(filepath.Join("web", "dist", "index.html")) {
		mux.Handle("/", spaFileServer(filepath.Join("web", "dist")))
	} else {
		mux.HandleFunc("/", s.handleRoot)
	}

	return recoverMiddleware(mux, s.log)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service":            "air-traffic",
		"message":            "Air-Traffic control plane API. Frontend ships in Phase 2.",
		"api":                []string{"/api/health", "/api/adapters", "/api/baselines", "/api/policies", "/api/observations", "/api/audit", "/api/drift", "/api/envconfig"},
		"synthetic_surfaces": "/synthetic/{vendor}/{native-path}",
	})
}

func recoverMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered", "error", rec, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func spaFileServer(root string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || !strings.Contains(filepath.Base(r.URL.Path), ".") {
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
