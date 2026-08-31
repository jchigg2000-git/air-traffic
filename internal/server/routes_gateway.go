package server

// Gateway ingest + config surfaces. Ingest is metadata-only by construction:
// the shared report types carry offsets/types/counts and the strict decoder
// (DisallowUnknownFields) rejects any payload smuggling a value field.

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

// gatewayStaleAfter is 3× the default heartbeat interval: one missed beat is
// jitter, three is an outage.
const gatewayStaleAfter = 45 * time.Second

func (s *Server) handleGatewayLeaks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var body struct {
		Reports []model.GatewayRequestReport `json:"reports"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid reports: "+err.Error())
		return
	}
	if len(body.Reports) == 0 {
		writeError(w, http.StatusBadRequest, "reports is empty")
		return
	}
	if len(body.Reports) > 1000 {
		writeError(w, http.StatusBadRequest, "max 1000 reports per push")
		return
	}
	for i := range body.Reports {
		clampReport(&body.Reports[i])
	}
	s.store.AddGatewayReports(body.Reports)

	// Blocks, leaks, and fail-mode trips surface on the Audit screen; routine
	// mask/pass traffic stays in the report ring (the harness's join source)
	// so a 2000-request run doesn't drown the audit stream.
	for _, rep := range body.Reports {
		if rep.Action != "block" && !rep.FailModeTripped {
			continue
		}
		s.store.AddAudit(model.AuditEvent{
			Actor: "air-traffic:gateway", Action: "gateway." + rep.Action,
			Resource: rep.Route, Plane: model.PlaneDataPolicy, Vendor: rep.Route,
			ControlSurface: model.DispProxyEnforced,
			After: map[string]any{
				"redaction_count":   len(rep.Redactions),
				"fail_mode_tripped": rep.FailModeTripped,
				"stream":            rep.Stream,
			},
			RequestID: rep.RequestID,
		})
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": len(body.Reports)})
}

// clampReport bounds string growth on the ingest path (a hostile or buggy
// gateway must not balloon the ring).
func clampReport(rep *model.GatewayRequestReport) {
	if len(rep.DetectorErrors) > 5 {
		rep.DetectorErrors = rep.DetectorErrors[:5]
	}
	for i, e := range rep.DetectorErrors {
		if len(e) > 300 {
			rep.DetectorErrors[i] = e[:300]
		}
	}
	if len(rep.Redactions) > 500 {
		rep.Redactions = rep.Redactions[:500]
	}
	for i, red := range rep.Redactions {
		if len(red.Path) > 200 {
			rep.Redactions[i].Path = red.Path[:200]
		}
	}
}

func (s *Server) handleGatewayEnforcement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var rep model.EnforcementReport
	if err := decodeJSON(r, &rep); err != nil {
		writeError(w, http.StatusBadRequest, "invalid report: "+err.Error())
		return
	}
	if rep.GatewayID == "" {
		writeError(w, http.StatusBadRequest, "gateway_id is required")
		return
	}
	rep.At = time.Now().UTC()
	s.store.SetGatewayEnforcement(rep)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGatewayPatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pack": s.store.GetPatternPack()})
}

// maxGatewayRequestPage bounds one page of the traffic feed. The ring itself
// holds more; a caller that wants everything pages for it.
const maxGatewayRequestPage = 1000

// handleGatewayRequests is the per-request traffic feed: what actually went
// through the proxy, newest first. Until this existed the reports the gateway
// pushed were only ever read by the harness scorer, so real application
// traffic left no visible trace anywhere in the UI.
//
// Metadata only, by construction — the reports carry redaction types, field
// paths, offsets and counts, never a redacted value. There is deliberately no
// cost field: the gateway reports the tokens the vendor billed and stops
// there (docs/plans/TODO-gateway-deferred.md).
func (s *Server) handleGatewayRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, maxGatewayRequestPage)
	}
	reports := s.store.ListGatewayReports(limit)
	if reports == nil {
		reports = []model.GatewayRequestReport{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requests": reports,
		"limit":    limit,
	})
}

// handleGatewayStatus is the UI's gateway liveness view — the browser never
// talks to the gateway port directly.
func (s *Server) handleGatewayStatus(w http.ResponseWriter, r *http.Request) {
	type gatewayStatus struct {
		GatewayID string `json:"gateway_id"`
		BaseURL   string `json:"base_url"`
		Action    string `json:"action"`
		// Detectors is the chain the gateway reports it is running — reported,
		// not assumed. A heartbeat from an older build carries none, and the UI
		// says so rather than defaulting to the regex floor.
		Detectors []string            `json:"detectors,omitempty"`
		Vendors   map[string][]string `json:"vendors"`
		LastSeen  time.Time           `json:"last_seen"`
		Fresh     bool                `json:"fresh"`
	}
	cutoff := time.Now().UTC().Add(-gatewayStaleAfter)
	out := []gatewayStatus{}
	for _, rep := range s.store.ListGatewayEnforcement() {
		out = append(out, gatewayStatus{
			GatewayID: rep.GatewayID, BaseURL: rep.BaseURL, Action: rep.Action,
			Detectors: rep.Detectors,
			Vendors:   rep.Vendors, LastSeen: rep.At, Fresh: rep.At.After(cutoff),
		})
	}
	// The auth posture is reported, not assumed: the honesty model applies to
	// our own control surface too — a viewer should be able to see that the
	// spine is loopback-gated or running an unrotated dev key.
	// A keystore that has silently stopped persisting looks healthy right up
	// until a restart drops every issued key, so its durability is reported
	// alongside the auth posture rather than left to be discovered.
	keystoreErr := ""
	if err := s.store.KeystorePersistError(); err != nil {
		keystoreErr = err.Error()
	}
	// Same reasoning for the applied policy: an apply that succeeded in memory
	// but failed to reach disk looks identical to one that worked, until the
	// restart that loses it.
	policyErr := ""
	if err := s.store.PolicyPersistError(); err != nil {
		policyErr = err.Error()
	}
	// And the pattern pack, whose failure mode is the same shape: the approved
	// rules are live in memory and on every gateway, and gone at the next
	// restart. Empty when no harness is attached — there is then nothing that
	// persists packs to fail.
	patternErr := ""
	if s.harness != nil {
		if err := s.harness.PatternPersistError(); err != nil {
			patternErr = err.Error()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"gateways":             out,
		"pattern_pack_version": s.store.GetPatternPack().Version,
		"pattern_error":        patternErr,
		"spine_auth":           s.spineAuthMode(),
		"spine_key_unrotated":  devSpineKeys[s.spineKey],
		"admin_auth":           s.adminAuthMode(),
		"keystore_version":     s.store.KeySnapshot().Version,
		"keystore_error":       keystoreErr,
		"policy_error":         policyErr,
	})
}
