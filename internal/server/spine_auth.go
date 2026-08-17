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

// SetAdminKey installs the operator key required on every state-changing
// control-plane route. Empty (the default) leaves those routes open, which is
// the posture this repo shipped with — see requireAdminWrite.
func (s *Server) SetAdminKey(key string) { s.adminKey = key }

// spineAuthMode names the posture in effect, for the status surface.
func (s *Server) spineAuthMode() string {
	if s.spineKey != "" {
		return "shared_key"
	}
	return "loopback_only"
}

// adminAuthMode names the write posture, for the status surface. It is
// reported rather than assumed because "open" is a real, reachable state and a
// control plane that quietly claims to be authenticated is the kind of lie
// this repo's honesty model exists to prevent.
func (s *Server) adminAuthMode() string {
	if s.adminKey != "" {
		return "admin_key"
	}
	return "open"
}

// requireAdminWrite gates the state-changing control-plane routes.
//
// This is the auth answer ratified 2026-08-15 ("The control plane stays
// single-operator; auth is the admin-key tier, not a user model"): the same
// two-tier ladder requireSpineKey implements, not an authenticated per-human
// principal. There is deliberately no user model, so an audit row can name the
// system but never a person.
//
// Reads pass through untouched. The observability surfaces are the product;
// gating them would break the SPA's whole purpose for a threat model that is
// one operator on one host.
//
// Posture:
//
//	AIRTRAFFIC_ADMIN_KEY set   → writes require it (Bearer or X-Air-Traffic-Admin-Key)
//	AIRTRAFFIC_ADMIN_KEY unset → writes are open; the boot logs say so out loud
//
// The unset case is open rather than loopback-only on purpose, and the reason
// is structural rather than a preference: compose publishes the control plane
// behind a port and serves the SPA from that same container, so a browser
// request arrives from the Docker bridge and is never loopback (the same fact
// routes_keystore.go documents). Loopback-only-when-unset would therefore 401
// the entire UI in the one-command demo stack. Keystore administration keeps
// its stricter loopback default because it mints credentials; these routes
// change configuration.
func (s *Server) requireAdminWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isReadMethod(r.Method) || s.adminKey == "" {
			next(w, r)
			return
		}
		if !validKey(r, s.adminKey) {
			writeError(w, http.StatusUnauthorized,
				"this route changes state and requires the operator key (X-Air-Traffic-Admin-Key)")
			return
		}
		next(w, r)
	}
}

// requireAdminIngest gates POST /api/observations, which has two legitimate
// writers with different credentials: an operator (admin key) and the gateway
// pushing its observation batches up the spine (spine key,
// internal/gateway/spine_emit.go). Either satisfies it. Folding this into
// requireAdminWrite would break the gateway; folding it into requireSpineKey
// would deny the operator.
func (s *Server) requireAdminIngest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isReadMethod(r.Method) {
			next(w, r)
			return
		}
		if s.adminKey != "" && validKey(r, s.adminKey) {
			next(w, r)
			return
		}
		s.requireSpineKey(next)(w, r)
	}
}

func isReadMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
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

func validSpineKey(r *http.Request, want string) bool { return validKey(r, want) }

// validKey compares any of the accepted key headers against want in constant
// time. The admin header is checked first so a caller holding both keys can
// present them unambiguously on a route that accepts either.
func validKey(r *http.Request, want string) bool {
	for _, h := range []string{"X-Air-Traffic-Admin-Key", "X-Air-Traffic-Key"} {
		if got := r.Header.Get(h); got != "" {
			if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
				return true
			}
		}
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
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
