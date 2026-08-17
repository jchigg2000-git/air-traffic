package server

import (
	"net/http"
	"strings"
	"time"

	"air-traffic/internal/audit"
	"air-traffic/internal/catalog"
	"air-traffic/internal/envconfig"
	"air-traffic/internal/model"
	"air-traffic/internal/policy"
	"air-traffic/internal/redact"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"service":       "air-traffic",
		"adapter_count": len(s.store.ListAdapters()),
		// Say which write posture is actually in effect. "open" is a real
		// state and naming it is the point — a control plane that implies it
		// is authenticated when it isn't is the exact class of claim this
		// repo's honesty model exists to prevent.
		"admin_auth": s.adminAuthMode(),
		"ts":         time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAdapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"adapters": s.store.ListAdapters()})
}

func (s *Server) handleAdapter(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/adapters/")
	parts := strings.Split(suffix, "/")
	id := parts[0]
	if id == "" {
		writeError(w, http.StatusNotFound, "missing adapter id")
		return
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch sub {
	case "":
		switch r.Method {
		case http.MethodGet:
			a, ok := s.store.GetAdapter(id)
			if !ok {
				writeError(w, http.StatusNotFound, "unknown adapter")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"adapter": a})
		case http.MethodPatch:
			var p model.AdapterPatch
			if err := decodeJSON(r, &p); err != nil {
				writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
				return
			}
			a, err := s.store.PatchAdapter(id, p)
			if err != nil {
				writeError(w, statusForError(err), err.Error())
				return
			}
			s.store.AddAudit(model.AuditEvent{Actor: "air-traffic:admin", Action: "adapter.patch", Resource: a.Vendor, Vendor: id, ControlSurface: model.DispVendorNative, RequestID: model.NewUUID()})
			writeJSON(w, http.StatusOK, map[string]any{"adapter": a})
		default:
			writeError(w, http.StatusMethodNotAllowed, "use GET or PATCH")
		}
	case "manifest":
		a, ok := s.store.GetAdapter(id)
		if !ok {
			writeError(w, http.StatusNotFound, "unknown adapter")
			return
		}
		def, _ := catalog.ByID(id)
		writeJSON(w, http.StatusOK, map[string]any{
			"adapter_id":    id,
			"vendor":        a.Vendor,
			"tier":          a.Tier,
			"capabilities":  a.Capabilities,
			"env_platforms": def.EnvPlatforms,
		})
	case "test":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		a, ok := s.store.GetAdapter(id)
		if !ok {
			writeError(w, http.StatusNotFound, "unknown adapter")
			return
		}
		st := model.Status{State: "healthy", Message: "synthetic surface reachable", CheckedAt: time.Now().UTC()}
		if !a.Enabled || a.Mode == model.ModeDisabled {
			st = model.Status{State: "degraded", Message: "adapter disabled", CheckedAt: time.Now().UTC()}
		} else if a.Scenario != "" && a.Scenario != "healthy" {
			st = model.Status{State: "degraded", Message: "scenario override active: " + a.Scenario, CheckedAt: time.Now().UTC()}
		}
		s.store.SetAdapterStatus(id, st)
		a, _ = s.store.GetAdapter(id)
		writeJSON(w, http.StatusOK, map[string]any{"adapter": a, "status": st})
	case "calls":
		writeJSON(w, http.StatusOK, map[string]any{"calls": s.store.ListCalls(id, limitParam(r, 200))})
	default:
		writeError(w, http.StatusNotFound, "unknown sub-resource")
	}
}

// baselineView is a baseline plus what applying it would actually do to
// gateway traffic. The derived fields are computed, never stored, and are the
// only thing that lets the Rigor Console tell an operator the consequence of
// Apply *before* it commits — previously the words detect/mask/block appeared
// nowhere on that page while being the only thing Apply changed.
//
// They hang off the response envelope rather than model.Baseline so the domain
// type stays a description of policy intent rather than of one enforcement
// point's behaviour.
type baselineView struct {
	model.Baseline
	// GatewayAction is what this baseline enforces WITHOUT the attestation.
	// Both derived fields are absolute properties of the baseline rather than
	// of the currently applied policy: the caller decides which one applies by
	// deciding whether it is attesting, and mixing the two — previewing
	// against current state while sending a different attestation — is exactly
	// how an operator silently drops an attestation that is already in force.
	GatewayAction string `json:"gateway_action"`
	// GatewayActionAttested is what it enforces WITH ZDR coverage attested.
	// Equal to GatewayAction for every baseline that does not gate on it.
	GatewayActionAttested string `json:"gateway_action_attested"`
	// RequiresZDRAttestation marks the baselines where those two differ — i.e.
	// where applying without the attestation blocks all traffic.
	RequiresZDRAttestation bool `json:"requires_zdr_attestation"`
}

func (s *Server) handleBaselines(w http.ResponseWriter, r *http.Request) {
	base := policy.Baselines()
	out := make([]baselineView, 0, len(base))
	for _, b := range base {
		out = append(out, baselineView{
			Baseline:               b,
			GatewayAction:          model.GatewayAction(b, false),
			GatewayActionAttested:  model.GatewayAction(b, true),
			RequiresZDRAttestation: model.RequiresZDRAttestation(b),
		})
	}
	// The attestation currently in force, so the UI can seed its checkbox from
	// reality instead of from a default that would revoke it.
	writeJSON(w, http.StatusOK, map[string]any{
		"baselines":    out,
		"zdr_attested": model.ZDRAttestedIn(s.store.GetPolicy()),
	})
}

func (s *Server) handlePolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"policy": s.store.GetPolicy()})
	case http.MethodPut:
		var p model.Policy
		if err := decodeJSON(r, &p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid policy: "+err.Error())
			return
		}
		if _, ok := policy.BaselineByID(p.Baseline); !ok && p.Baseline != "" {
			writeError(w, http.StatusBadRequest, "unknown baseline: "+p.Baseline)
			return
		}
		report := policy.Apply(s.store, p)
		policy.RefreshDrift(s.store, time.Now().UTC())
		writeJSON(w, http.StatusOK, map[string]any{"coverage": report})
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or PUT")
	}
}

func (s *Server) handleCredentials(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"credentials": s.store.ListCredentials()})
	case http.MethodPost:
		var raw map[string]any
		if err := decodeJSON(r, &raw); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		if redact.HasPlaintextSecretKey(raw) {
			writeError(w, http.StatusBadRequest, "plaintext secrets are not accepted; send secret_ref only")
			return
		}
		cred := model.Credential{
			Name:      str(raw["name"]),
			Provider:  str(raw["provider"]),
			Type:      str(raw["type"]),
			SecretRef: str(raw["secret_ref"]),
		}
		if cred.SecretRef == "" {
			writeError(w, http.StatusBadRequest, "secret_ref is required")
			return
		}
		created := s.store.AddCredential(cred)
		writeJSON(w, http.StatusCreated, map[string]any{"credential": created})
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
	}
}

func (s *Server) handleObservations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"observations": s.store.ListObservations(limitParam(r, 200))})
	case http.MethodPost:
		var batch map[string]any
		if err := decodeJSON(r, &batch); err != nil {
			writeError(w, http.StatusBadRequest, "invalid batch: "+err.Error())
			return
		}
		if batch["contract"] != model.ObservationContract {
			writeError(w, http.StatusBadRequest, "contract must be "+model.ObservationContract)
			return
		}
		conn, _ := batch["connector"].(map[string]any)
		rec := s.store.AddObservation(model.ObservationRecord{
			Contract:          model.ObservationContract,
			ConnectorType:     str(conn["type"]),
			ConnectorInstance: str(conn["instance"]),
			Complete:          boolOf(batch["complete"]),
			Body:              batch,
		})
		writeJSON(w, http.StatusAccepted, map[string]any{"observation": rec})
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
	}
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	events := s.store.ListAudit(limitParam(r, 200))
	if r.URL.Query().Get("format") == "siem" {
		writeJSON(w, http.StatusOK, map[string]any{"records": audit.ToSIEM(events)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit": events})
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"activity": s.store.ListActivity(limitParam(r, 200))})
}

func (s *Server) handleDrift(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"drift": s.store.ListDrift()})
}

// handleCostFacets exposes the per-vendor cost drill-down CONFIG: which dimensions each
// vendor's real billing/usage API can break cost down by (supported) and which it cannot
// (unsupported, with an honest reason). The Cost Explorer pairs this metadata with the
// per-member VALUES carried in the ops-observation-batch/v1 stream (kind "cost_breakdown").
func (s *Server) handleCostFacets(w http.ResponseWriter, r *http.Request) {
	type facetMeta struct {
		Dimension     string `json:"dimension"`
		Label         string `json:"label"`
		RealParam     string `json:"real_param,omitempty"`
		Endpoint      string `json:"endpoint,omitempty"`
		ResponseField string `json:"response_field,omitempty"`
		Reason        string `json:"reason,omitempty"`
	}
	type vendorFacets struct {
		Vendor      string      `json:"vendor"`
		Supported   []facetMeta `json:"supported"`
		Unsupported []facetMeta `json:"unsupported"`
	}
	out := map[string]vendorFacets{}
	for _, def := range catalog.All() {
		if len(def.CostFacets) == 0 {
			continue
		}
		vf := vendorFacets{Vendor: def.DisplayName, Supported: []facetMeta{}, Unsupported: []facetMeta{}}
		for _, f := range def.CostFacets {
			m := facetMeta{Dimension: f.Dimension, Label: f.Label, RealParam: f.RealParam, Endpoint: f.Endpoint, ResponseField: f.ResponseField, Reason: f.Reason}
			if f.Supported && len(f.Members) > 0 {
				vf.Supported = append(vf.Supported, m)
			} else {
				vf.Unsupported = append(vf.Unsupported, m)
			}
		}
		out[def.ID] = vf
	}
	writeJSON(w, http.StatusOK, map[string]any{"facets": out})
}

func (s *Server) handleEnvConfig(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	var artifacts []model.ManagedArtifact
	var states []model.EnvState
	for _, a := range s.store.ListAdapters() {
		def, _ := catalog.ByID(a.ID)
		for _, ep := range def.EnvPlatforms {
			if platform != "" && ep.Platform != platform {
				continue
			}
			art := envconfig.Render(ep.Platform, a.ID, nil, ep.Enforcement)
			artifacts = append(artifacts, art)
			states = append(states, envconfig.ReadState(ep.Platform, a.ID, art, false))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts, "states": states})
}

// ---- small helpers ----

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func boolOf(v any) bool {
	b, _ := v.(bool)
	return b
}
