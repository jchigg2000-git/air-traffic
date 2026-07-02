// Package synthetic serves byte-identical replicas of each vendor's admin/control
// surface at /synthetic/{vendor}/{native-path}, plus the Air-Traffic harness control
// paths. Mirrors it-scorecard's synthetic handler, extended to AI-vendor admin APIs.
package synthetic

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"air-traffic/internal/catalog"
	"air-traffic/internal/model"
	"air-traffic/internal/redact"
	"air-traffic/internal/store"
)

// Handler routes /synthetic/* requests.
type Handler struct {
	store *store.Store
	log   *slog.Logger
}

func New(st *store.Store, log *slog.Logger) *Handler { return &Handler{store: st, log: log} }

// FixtureFunc produces a byte-identical success body for one vendor path.
// Returns HTTP status, the JSON-serializable body, and extra response headers.
type FixtureFunc func(a model.Adapter, method, path string, query map[string][]string) (int, any, map[string]string)

// fixtures is the per-vendor registry of byte-identical generators.
var fixtures = map[string]FixtureFunc{}

func register(vendorID string, f FixtureFunc) { fixtures[vendorID] = f }

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(c int) { r.code = c; r.ResponseWriter.WriteHeader(c) }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/synthetic/")
	if rest == "" || rest == r.URL.Path {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "missing adapter id"})
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	adapterID := parts[0]
	nativePath := "/"
	if len(parts) == 2 {
		nativePath = "/" + parts[1]
	}

	a, ok := h.store.GetAdapter(adapterID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown adapter", "adapter_id": adapterID})
		return
	}

	// Harness control sub-paths (Air-Traffic control, not part of the vendor API).
	if strings.HasPrefix(nativePath, "/_harness") {
		h.handleHarness(w, r, a, nativePath)
		return
	}

	// The inference upstream sits before the mode/proxy checks so flipping
	// adapter mode never breaks the gateway's mock target (it still honors
	// scenario overrides internally). It needs the raw ResponseWriter for SSE.
	if isInferencePath(a.ID, nativePath) {
		h.handleInference(w, r, a, nativePath)
		return
	}

	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
	defer func() {
		h.store.RecordCall(model.CallRecord{
			AdapterID:  a.ID,
			Method:     r.Method,
			Path:       nativePath,
			Query:      redact.Query(r.URL.Query()),
			Headers:    redact.Headers(r.Header),
			Scenario:   a.Scenario,
			StatusCode: rec.code,
			DurationMS: time.Since(start).Milliseconds(),
		})
	}()

	// disabled → vendor-shaped 503
	if !a.Enabled || a.Mode == model.ModeDisabled {
		status, body := vendorError(a.ID, http.StatusServiceUnavailable, "service_unavailable", "adapter disabled")
		writeJSON(rec, status, body)
		return
	}

	// scenario overrides take precedence and emit the vendor's own error envelope
	if handled := h.writeScenario(rec, a); handled {
		return
	}

	// proxy mode is stubbed in Phase 1 (gateway is out of scope)
	if a.Mode == model.ModeProxy {
		status, body := vendorError(a.ID, http.StatusNotImplemented, "proxy_not_normalized", "proxy mode reached upstream but no vendor normalizer is implemented in Phase 1")
		writeJSON(rec, status, body)
		return
	}

	// synthetic mode → byte-identical fixture
	f, ok := fixtures[a.ID]
	if !ok {
		f = genericFixture
	}
	status, body, headers := f(a, r.Method, nativePath, r.URL.Query())
	for k, v := range headers {
		rec.Header().Set(k, v)
	}
	writeJSON(rec, status, body)
}

func (h *Handler) writeScenario(w http.ResponseWriter, a model.Adapter) bool {
	switch a.Scenario {
	case "401":
		s, b := vendorError(a.ID, http.StatusUnauthorized, "unauthorized", "synthetic unauthorized")
		writeJSON(w, s, b)
	case "403":
		s, b := vendorError(a.ID, http.StatusForbidden, "forbidden", "synthetic forbidden")
		writeJSON(w, s, b)
	case "429-retry-after":
		w.Header().Set("Retry-After", "30")
		s, b := vendorError(a.ID, http.StatusTooManyRequests, "rate_limit_exceeded", "synthetic rate limit")
		writeJSON(w, s, b)
	case "500":
		s, b := vendorError(a.ID, http.StatusInternalServerError, "server_error", "synthetic server error")
		writeJSON(w, s, b)
	case "503":
		s, b := vendorError(a.ID, http.StatusServiceUnavailable, "service_unavailable", "synthetic unavailable")
		writeJSON(w, s, b)
	case "timeout":
		time.Sleep(1500 * time.Millisecond)
		s, b := vendorError(a.ID, http.StatusGatewayTimeout, "timeout", "synthetic timeout")
		writeJSON(w, s, b)
	case "invalid-json":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"object":"list","data":[`)) // deliberately truncated
	case "empty":
		writeJSON(w, http.StatusOK, emptyEnvelope(a.ID))
	default:
		return false
	}
	return true
}

func (h *Handler) handleHarness(w http.ResponseWriter, r *http.Request, a model.Adapter, nativePath string) {
	switch {
	case strings.HasPrefix(nativePath, "/_harness/manifest"):
		def, _ := catalog.ByID(a.ID)
		writeJSON(w, http.StatusOK, map[string]any{
			"adapter_id":          a.ID,
			"vendor":              a.Vendor,
			"mode":                a.Mode,
			"tier":                a.Tier,
			"supported_modes":     []string{"disabled", "synthetic", "proxy"},
			"supported_scenarios": []string{"healthy", "401", "403", "429-retry-after", "500", "503", "timeout", "invalid-json", "empty"},
			"capabilities":        a.Capabilities,
			"env_platforms":       def.EnvPlatforms,
		})
	case strings.HasPrefix(nativePath, "/_harness/scenario"):
		if r.Method != http.MethodPut {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "use PUT"})
			return
		}
		name := strings.TrimPrefix(nativePath, "/_harness/scenario/")
		if name == "/_harness/scenario" || name == nativePath {
			var body struct {
				Scenario string `json:"scenario"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			name = body.Scenario
		}
		updated, err := h.store.SetScenario(a.ID, name)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"adapter": updated})
	case strings.HasPrefix(nativePath, "/_harness/reset"):
		h.store.ResetCalls(a.ID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case strings.HasPrefix(nativePath, "/_harness/calls"):
		writeJSON(w, http.StatusOK, map[string]any{"calls": h.store.ListCalls(a.ID, 200)})
	case strings.HasPrefix(nativePath, "/_harness/inference"):
		writeJSON(w, http.StatusOK, map[string]any{"captures": h.store.ListInferenceCaptures(a.ID, 200)})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown harness path", "path": nativePath})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(body)
}
