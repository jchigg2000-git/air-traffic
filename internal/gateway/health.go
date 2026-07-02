package gateway

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeHealth(w, http.StatusOK, map[string]any{"status": "ok"})
}

// handleReadyz reports whether the gateway can do useful work: upstream routes
// configured and client keys resolved. Later phases append detector-chain and
// policy-snapshot readiness here.
func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	var reasons []string
	if len(s.cfg.Upstreams) == 0 {
		reasons = append(reasons, "no upstreams configured")
	}
	if len(s.clientKeys) == 0 {
		reasons = append(reasons, "no client keys resolved")
	}
	if len(reasons) > 0 {
		writeHealth(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "reasons": reasons})
		return
	}
	writeHealth(w, http.StatusOK, map[string]any{"status": "ready"})
}

func writeHealth(w http.ResponseWriter, code int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
