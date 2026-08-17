package server

// Keystore admin surface: register apps, issue keys against them, revoke.
//
// This is the only credential-minting surface in the system, so it is gated
// harder than anything else here — see requireLocalAdmin. Everything it
// returns is metadata except the single issuance response, which carries the
// plaintext key exactly once and is never recoverable afterwards.

import (
	"net/http"
	"time"

	"air-traffic/internal/model"
	"air-traffic/internal/policy"
)

// requireLocalAdmin restricts a route to callers on this host.
//
// Deliberately NOT gated by AIRTRAFFIC_SPINE_KEY: the gateway holds that key,
// and a gateway that can push leak reports must not thereby be able to mint
// itself a credential. Those are two different trust boundaries and collapsing
// them would make the keystore pointless.
//
// Two ways in, either sufficient: a loopback caller, or one presenting
// AIRTRAFFIC_ADMIN_KEY. The key tier was added 2026-08-16 (ratified
// 2026-08-15) precisely because loopback alone is unreachable from the SPA —
// in the compose stack the browser request arrives from the Docker bridge, not
// from loopback, which is why scripts/keystore.sh has to route its call
// through the container's netns. With the key set, a keystore UI becomes
// buildable; without it the loopback-only posture is unchanged.
func (s *Server) requireLocalAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminKey != "" && validKey(r, s.adminKey) {
			next(w, r)
			return
		}
		if !isLoopback(r.RemoteAddr) {
			msg := "keystore administration accepts loopback callers only"
			if s.adminKey != "" {
				msg = "keystore administration requires loopback or the operator key (X-Air-Traffic-Admin-Key)"
			}
			writeError(w, http.StatusUnauthorized, msg)
			return
		}
		next(w, r)
	}
}

// ---- apps ----

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"apps": s.store.ListApps()})
	case http.MethodPost:
		var body struct {
			ID       string `json:"id"`
			Name     string `json:"name,omitempty"`
			Baseline string `json:"baseline,omitempty"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid app: "+err.Error())
			return
		}
		if !validBaseline(body.Baseline) {
			writeError(w, http.StatusBadRequest, "unknown baseline "+body.Baseline)
			return
		}
		app, err := s.store.CreateApp(model.App{ID: body.ID, Name: body.Name, Baseline: body.Baseline})
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		s.auditKeystore("keystore.app.create", app.ID, map[string]any{
			"name": app.Name, "baseline": app.Baseline,
		})
		writeJSON(w, http.StatusCreated, map[string]any{"app": app})
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
	}
}

func (s *Server) handleApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	switch r.Method {
	case http.MethodGet:
		app, ok := s.store.GetApp(id)
		if !ok {
			writeError(w, http.StatusNotFound, "no app "+id)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"app": app})
	case http.MethodPatch:
		// Pointers so "absent" and "cleared" stay distinguishable: passing
		// baseline:"" is how an app is returned to the global policy.
		var body struct {
			Name     *string `json:"name,omitempty"`
			Baseline *string `json:"baseline,omitempty"`
			Disabled *bool   `json:"disabled,omitempty"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid patch: "+err.Error())
			return
		}
		if body.Baseline != nil && !validBaseline(*body.Baseline) {
			writeError(w, http.StatusBadRequest, "unknown baseline "+*body.Baseline)
			return
		}
		app, err := s.store.UpdateApp(id, body.Baseline, body.Disabled, body.Name)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		s.auditKeystore("keystore.app.update", app.ID, map[string]any{
			"baseline": app.Baseline, "disabled": app.Disabled,
		})
		writeJSON(w, http.StatusOK, map[string]any{"app": app})
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or PATCH")
	}
}

// ---- keys ----

func (s *Server) handleAppKeys(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	if _, ok := s.store.GetApp(appID); !ok {
		writeError(w, http.StatusNotFound, "no app "+appID)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"keys": s.store.ListKeys(appID)})
	case http.MethodPost:
		var body struct {
			Subject       string   `json:"subject,omitempty"`
			Label         string   `json:"label,omitempty"`
			Routes        []string `json:"routes,omitempty"`
			ExpiresInDays int      `json:"expires_in_days,omitempty"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid key request: "+err.Error())
			return
		}
		for _, rt := range body.Routes {
			if rt != "anthropic" && rt != "openai" {
				writeError(w, http.StatusBadRequest, "unknown route "+rt+" (want anthropic or openai)")
				return
			}
		}
		var expiresAt *time.Time
		if body.ExpiresInDays > 0 {
			t := time.Now().UTC().AddDate(0, 0, body.ExpiresInDays)
			expiresAt = &t
		}
		key, plaintext, err := s.store.IssueKey(appID, body.Subject, body.Label, body.Routes, expiresAt)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		// The audit event records that a key was minted and for whom. It does
		// not, and must not, record the key.
		s.auditKeystore("keystore.key.issue", appID, map[string]any{
			"key_id": key.ID, "subject": key.Subject, "routes": key.Routes,
			"expires_at": key.ExpiresAt,
		})
		key.Digest = ""
		writeJSON(w, http.StatusCreated, map[string]any{
			"key":    key,
			"secret": plaintext,
			"notice": "this is the only time the key is shown; it is stored as a digest and cannot be recovered",
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
	}
}

func (s *Server) handleKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "use DELETE")
		return
	}
	key, err := s.store.RevokeKey(r.PathValue("kid"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	s.auditKeystore("keystore.key.revoke", key.AppID, map[string]any{
		"key_id": key.ID, "subject": key.Subject,
	})
	key.Digest = ""
	writeJSON(w, http.StatusOK, map[string]any{
		"key": key,
		"notice": "revocation reaches gateways on their next keystore pull " +
			"(GATEWAY_POLICY_PULL_INTERVAL, 15s by default)",
	})
}

// handleGatewayKeys distributes the keystore snapshot to gateways. It rides
// the spine auth already covering /api/gateway/*, and ships digests rather
// than secrets, so a leaked snapshot yields no usable credential.
func (s *Server) handleGatewayKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshot": s.store.KeySnapshot()})
}

// validBaseline accepts "" (inherit the global policy) or a known baseline id.
func validBaseline(id string) bool {
	if id == "" {
		return true
	}
	_, ok := policy.BaselineByID(id)
	return ok
}

func (s *Server) auditKeystore(action, appID string, after map[string]any) {
	s.store.AddAudit(model.AuditEvent{
		Actor:          "air-traffic:admin",
		Action:         action,
		Resource:       "app/" + appID,
		Plane:          model.PlaneDataPolicy,
		ControlSurface: model.DispProxyEnforced,
		After:          after,
	})
}
