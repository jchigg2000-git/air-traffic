package server

// Auth for the spine routes the gateway drives (build plan §4.2). These are
// not UI routes: POST /api/gateway/leaks and /enforcement ingest enforcement
// evidence, and GET /api/gateway/patterns *distributes* the pattern pack —
// which since the G6 config-knob slice carries deny-list TERMS. Synthetic
// here; real names in a real deployment. So the read side is guarded too.
//
// Posture, in order of preference:
//
//	AIRTRAFFIC_SPINE_KEY set  → shared-key required (Bearer or X-Air-Traffic-Key)
//	AIRTRAFFIC_SPINE_KEY unset → loopback callers only; everyone else 401
//
// The unset case is the dev posture the gateway shipped with, narrowed from
// "anyone who can reach the port" to "anyone on this host". It is not a
// substitute for the key on any shared deployment — a container-network peer
// (the compose gateway) is already not loopback, so compose sets the key.

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// devSpineKeys are the throwaway values shipped in docker-compose so the demo
// stack comes up with one command. Presence of one of these is reported as
// unrotated — see cmd/air-traffic-server/main.go.
var devSpineKeys = map[string]bool{
	"spine-dev-insecure": true,
	"gwk-demo":           true,
}

// SetSpineKey installs the shared key the gateway must present on the spine
// routes. Empty (the default) leaves the loopback-only posture in place.
func (s *Server) SetSpineKey(key string) { s.spineKey = key }

// spineAuthMode names the posture in effect, for the status surface.
func (s *Server) spineAuthMode() string {
	if s.spineKey != "" {
		return "shared_key"
	}
	return "loopback_only"
}

// requireSpineKey gates a spine route per the posture above.
func (s *Server) requireSpineKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.spineKey != "" {
			if !validSpineKey(r, s.spineKey) {
				writeError(w, http.StatusUnauthorized, "invalid or missing gateway spine key")
				return
			}
			next(w, r)
			return
		}
		if !isLoopback(r.RemoteAddr) {
			writeError(w, http.StatusUnauthorized,
				"spine routes accept non-local callers only with AIRTRAFFIC_SPINE_KEY configured")
			return
		}
		next(w, r)
	}
}

func validSpineKey(r *http.Request, want string) bool {
	got := r.Header.Get("X-Air-Traffic-Key")
	if got == "" {
		got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// isLoopback reports whether a RemoteAddr came from this host. An unparseable
// address is treated as remote — fail closed.
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
