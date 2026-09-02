// Package gateway implements the inference-gateway data plane: a reverse
// proxy speaking vendor API dialects with an inline PII detector chain,
// governed by (but deployed separately from) the Air-Traffic control plane.
package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/jchigg2000-git/air-traffic/internal/gateway/config"
	"github.com/jchigg2000-git/air-traffic/internal/gateway/credbroker"
	"github.com/jchigg2000-git/air-traffic/internal/gateway/detect"
	"github.com/jchigg2000-git/air-traffic/internal/model"
)

// Server is one gateway instance. It holds no cross-request state; everything
// mutable is either per-request or an atomically-swapped snapshot.
type Server struct {
	cfg        config.Config
	log        *slog.Logger
	creds      credbroker.Resolver
	clientKeys map[string]struct{}
	// spineKey is presented to the control plane on push/pull. Empty when no
	// secret is configured — the control plane then accepts this gateway only
	// if it reaches it over loopback.
	spineKey string

	chain    *detect.Chain
	regexDet *detect.Regex
	presidio *detect.Presidio // nil unless in the chain

	// policyAction holds the action derived from the pulled control-plane
	// policy when GATEWAY_REDACT_ACTION=per_policy. policyBaseline names the
	// baseline it came from, and zdrAttested is the input deriveAction needs
	// to resolve a per-app baseline the same way.
	policyAction   atomic.Value
	policyBaseline atomic.Value
	zdrAttested    atomic.Bool
	packVersion    atomic.Int64

	// baselines is every baseline the control plane publishes, by id, so a
	// per-app baseline can be derived without a fetch per request.
	baselines atomic.Pointer[map[string]model.Baseline]
	// keys is the pulled keystore. nil until the first successful pull; the
	// env key set carries authentication until then.
	keys atomic.Pointer[keySnapshot]

	audits  auditRing
	metrics *metrics
}

// New validates runtime wiring (client keys resolvable, upstreams present,
// detector chain compiled) and returns a ready-to-serve gateway.
func New(cfg config.Config, log *slog.Logger) (*Server, error) {
	creds := credbroker.New()
	raw, err := creds.Resolve(cfg.ClientKeysRef)
	if err != nil {
		return nil, fmt.Errorf("resolving GATEWAY_CLIENT_KEYS_REF: %w", err)
	}
	keys := make(map[string]struct{})
	for _, k := range strings.Split(raw, ",") {
		if k = strings.TrimSpace(k); k != "" {
			keys[k] = struct{}{}
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("GATEWAY_CLIENT_KEYS_REF resolved to zero client keys")
	}

	// The spine key is optional: an env: ref that resolves to nothing means
	// "unauthenticated spine", which only works against a loopback control
	// plane. A vault:/kms: ref that fails is a real misconfiguration.
	spineKey, err := creds.Resolve(cfg.ControlPlaneKeyRef)
	if err != nil {
		if !strings.HasPrefix(cfg.ControlPlaneKeyRef, "env:") {
			return nil, fmt.Errorf("resolving GATEWAY_CONTROL_PLANE_KEY_REF: %w", err)
		}
		log.Warn("no control-plane spine key configured; the control plane will accept this gateway's pushes only over loopback",
			"ref", cfg.ControlPlaneKeyRef)
		spineKey = ""
	}

	s := &Server{cfg: cfg, log: log, creds: creds, clientKeys: keys, spineKey: spineKey, metrics: newMetrics()}
	var detectors []detect.Detector
	for _, name := range cfg.Detectors {
		switch name {
		case "regex":
			s.regexDet = detect.NewRegex()
			detectors = append(detectors, s.regexDet)
		case "presidio":
			s.presidio = detect.NewPresidio(cfg.PresidioURL)
			detectors = append(detectors, s.presidio)
		}
	}
	s.chain = &detect.Chain{Detectors: detectors, Timeout: cfg.DetectorTimeout}
	return s, nil
}

// Routes returns the gateway's handler tree.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("POST /v1/messages", s.requireClientKey(anthropicDialect(), s.handleMessages))
	mux.HandleFunc("POST /v1/chat/completions", s.requireClientKey(openAIDialect(), s.handleChatCompletions))
	return s.recoverMiddleware(s.requestID(mux))
}

// requestID assigns (or propagates) the correlation ID the harness and audit
// pipeline join on. It is echoed on the response so callers can retrieve it.
func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Gateway-Request-Id")
		if !validRequestID(id) {
			id = newRequestID()
			r.Header.Set("X-Gateway-Request-Id", id)
		}
		w.Header().Set("X-Gateway-Request-Id", id)
		next.ServeHTTP(w, r)
	})
}

// validRequestID accepts a caller-supplied correlation ID only when it is
// short and printable ASCII: it is echoed on the response, logged on every
// request, and carried in every report, so it gets the same bounds as any
// other string that crosses those surfaces. Anything else is replaced.
func validRequestID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered", "path", r.URL.Path, "panic", fmt.Sprint(rec))
				http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return "gw-" + hex.EncodeToString(b)
}
