// Package server wires the Air-Traffic HTTP API, the synthetic vendor surfaces,
// and the SPA fallback (Phase 2). Stdlib-only.
package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/jchigg2000-git/air-traffic/internal/audit"
	"github.com/jchigg2000-git/air-traffic/internal/harness"
	"github.com/jchigg2000-git/air-traffic/internal/store"
	"github.com/jchigg2000-git/air-traffic/internal/synthetic"
)

// Server holds dependencies for the HTTP handlers.
type Server struct {
	store     *store.Store
	log       *slog.Logger
	synthetic *synthetic.Handler
	harness   *harness.Runner
	// spineKey is the shared secret the gateway presents on /api/gateway/*
	// ingest + pattern reads. Empty means loopback-only (see spine_auth.go).
	spineKey string
	// adminKey gates every state-changing control-plane route. Empty leaves
	// them open — the posture this repo shipped with, reported honestly rather
	// than silently (see requireAdminWrite).
	adminKey string
}

// New constructs a Server and seeds the audit stream.
func New(st *store.Store, log *slog.Logger) *Server {
	for _, e := range audit.Seed() {
		st.AddAudit(e)
	}
	srv := &Server{store: st, log: log, synthetic: synthetic.New(st, log)}
	// The two mutating /synthetic/{vendor}/_harness paths write the same
	// adapter record PATCH /api/adapters/{id} does, so they carry the same
	// key. The closure reads adminKey at request time, so SetAdminKey may
	// still be called after New.
	srv.synthetic.SetAdminGuard(func(r *http.Request) bool {
		return srv.adminKey == "" || validKey(r, srv.adminKey)
	})
	return srv
}

// SetHarness attaches the optional harness engine (keeps New's signature for
// existing callers; /api/harness/* 503s without it).
func (s *Server) SetHarness(h *harness.Runner) { s.harness = h }

// Routes returns the fully wired handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Reads are ungated; every state-changing route below carries
	// requireAdminWrite (spine_auth.go), which is a no-op until
	// AIRTRAFFIC_ADMIN_KEY is set. GET-only routes are left bare rather than
	// wrapped in a gate that could never fire.
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/adapters", s.handleAdapters)
	mux.HandleFunc("/api/adapters/", s.requireAdminWrite(s.handleAdapter))
	mux.HandleFunc("/api/baselines", s.handleBaselines)
	mux.HandleFunc("/api/policies", s.requireAdminWrite(s.handlePolicies))
	mux.HandleFunc("/api/credentials", s.requireAdminWrite(s.handleCredentials))
	// Two legitimate writers, two different credentials — see requireAdminIngest.
	mux.HandleFunc("/api/observations", s.requireAdminIngest(s.handleObservations))
	mux.HandleFunc("/api/audit", s.handleAudit)
	mux.HandleFunc("/api/activity", s.handleActivity)
	mux.HandleFunc("/api/drift", s.handleDrift)
	mux.HandleFunc("/api/cost/facets", s.handleCostFacets)
	mux.HandleFunc("/api/envconfig", s.handleEnvConfig)
	// Spine routes (gateway ↔ control plane), key- or loopback-gated. /status
	// stays open: it is the browser's liveness view and carries no terms.
	mux.HandleFunc("/api/gateway/leaks", s.requireSpineKey(s.handleGatewayLeaks))
	mux.HandleFunc("/api/gateway/enforcement", s.requireSpineKey(s.handleGatewayEnforcement))
	mux.HandleFunc("/api/gateway/patterns", s.requireSpineKey(s.handleGatewayPatterns))
	mux.HandleFunc("/api/gateway/keys", s.requireSpineKey(s.handleGatewayKeys))
	mux.HandleFunc("/api/gateway/status", s.handleGatewayStatus)
	mux.HandleFunc("/api/gateway/requests", s.handleGatewayRequests)
	// Keystore administration — the only credential-minting surface here, so
	// it is loopback-gated rather than sharing the spine key (routes_keystore.go).
	mux.HandleFunc("/api/apps", s.requireLocalAdmin(s.handleApps))
	mux.HandleFunc("/api/apps/{id}", s.requireLocalAdmin(s.handleApp))
	mux.HandleFunc("/api/apps/{id}/keys", s.requireLocalAdmin(s.handleAppKeys))
	mux.HandleFunc("/api/keys/{kid}", s.requireLocalAdmin(s.handleKey))
	mux.HandleFunc("/api/harness/runs", s.requireAdminWrite(s.handleHarnessRuns))
	mux.HandleFunc("/api/harness/runs/", s.handleHarnessRun)
	mux.HandleFunc("/api/harness/sample", s.requireAdminWrite(s.handleHarnessSample))
	mux.HandleFunc("/api/harness/ratchet", s.handleHarnessRatchet)
	mux.HandleFunc("/api/harness/corpus", s.handleHarnessCorpus)
	// Proposal approve/reject is the one irreversible control in the product:
	// an approved allow_list rule removes detection and there is no retraction
	// path yet (ROADMAP.md PIVOT-7).
	mux.HandleFunc("/api/harness/proposals", s.requireAdminWrite(s.handleHarnessProposals))
	mux.HandleFunc("/api/harness/proposals/", s.requireAdminWrite(s.handleHarnessProposals))

	// The vendor replica answers anyone by design — it serves fabricated data.
	// The synthetic Anthropic upstream is the one surface that retains input
	// (a 64 KB-bounded capture of each request body, readable at
	// _harness/inference; see SECURITY.md). Its two mutating control paths
	// (_harness/scenario, _harness/reset) carry the operator key via
	// SetAdminGuard in New.
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
		"service": "air-traffic",
		"message": "Air-Traffic control plane API. The SPA is not built here — web/dist is gitignored. " +
			"Run `cd web && npm install && npm run build` from the repo root and reload, or " +
			"`docker compose up -d --build`, which bakes it in.",
		"api":                []string{"/api/health", "/api/adapters", "/api/activity", "/api/baselines", "/api/policies", "/api/observations", "/api/audit", "/api/drift", "/api/envconfig", "/api/cost/facets"},
		"api_reference":      "README.md#http-api",
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
