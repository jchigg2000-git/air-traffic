package policy

import (
	"time"

	"air-traffic/internal/envconfig"
	"air-traffic/internal/model"
	"air-traffic/internal/store"
)

// RefreshDrift re-reads the effective state of every env_managed surface, compares
// it to declared intent, replaces the drift set in the store, and emits a drift
// observation for each divergence. Deterministic so tests can assert on it.
func RefreshDrift(st *store.Store, ts time.Time) {
	var records []model.DriftRecord
	for _, a := range st.ListAdapters() {
		if !a.Enabled || a.Mode == model.ModeDisabled {
			continue
		}
		for _, c := range a.Capabilities {
			if c.Disposition != model.DispEnvManaged {
				continue
			}
			overridden := overrideHeuristic(a.ID, c.Key)
			platform := platformFor(c.Enforcement)
			art := envconfig.Render(platform, a.ID, nil, c.Enforcement)
			state := envconfig.ReadState(platform, a.ID, art, overridden)
			if !state.DriftDetected {
				continue
			}
			records = append(records, model.DriftRecord{
				Vendor: a.ID, Capability: c.Key, Plane: c.Plane, Surface: c.Disposition,
				Declared: "locked", Actual: state.Source, Severity: "warning",
				Message: state.DriftMessage, DetectedAt: ts,
			})
			emitDriftObs(st, a, c, state, ts)
		}
	}
	records = append(records, gatewayDrift(st, ts)...)
	st.ReplaceDrift(records)
}

// gatewayDrift surfaces proxy_enforced divergence: a capability a gateway was
// enforcing whose heartbeat went stale is an error (enforcement silently
// stopped); a policy that expects PII redaction with no gateway reporting at
// all is a warning (the label is aspirational).
func gatewayDrift(st *store.Store, ts time.Time) []model.DriftRecord {
	expected := false
	if p := st.GetPolicy(); p != nil {
		if b, ok := BaselineByID(p.Baseline); ok {
			expected = b.PIIRedaction != "off"
		}
	}
	var records []model.DriftRecord
	for _, a := range st.ListAdapters() {
		if !a.Enabled || a.Mode == model.ModeDisabled {
			continue
		}
		for _, c := range a.Capabilities {
			if c.Disposition != model.DispProxyEnforced {
				continue
			}
			_, fresh, everSeen := st.GatewayEnforcement(a.ID, c.Key, gatewayStaleAfter)
			if fresh {
				continue
			}
			switch {
			case everSeen:
				records = append(records, model.DriftRecord{
					Vendor: a.ID, Capability: c.Key, Plane: c.Plane, Surface: c.Disposition,
					Declared: "proxy_enforced", Actual: "unenforced (gateway stale)", Severity: "error",
					Message: "gateway enforcement heartbeat for " + c.Key + " went stale; requests are no longer filtered",
					DetectedAt: ts,
				})
				emitGatewayDriftObs(st, a, c, "red", "error", ts)
			case expected && c.Key == "pii_redaction":
				records = append(records, model.DriftRecord{
					Vendor: a.ID, Capability: c.Key, Plane: c.Plane, Surface: c.Disposition,
					Declared: "proxy_enforced (policy intent)", Actual: "no gateway reporting", Severity: "warning",
					Message: "applied baseline expects PII redaction but no gateway has reported enforcement",
					DetectedAt: ts,
				})
				emitGatewayDriftObs(st, a, c, "amber", "warning", ts)
			}
		}
	}
	return records
}

func emitGatewayDriftObs(st *store.Store, a model.Adapter, c model.Capability, status, severity string, ts time.Time) {
	obs := model.Obs("state", c.Key+"_gateway_drift", 0, "bool", status, severity, c.Plane, a.ID, c.Disposition,
		map[string]any{"source": "gateway_heartbeat"}, "gateway", "")
	body := model.BuildBatch(a, ts, time.Minute, []any{obs}, nil)
	st.AddObservation(model.ObservationRecord{
		ReceivedAt: ts, Contract: model.ObservationContract, ConnectorType: "ai-vendor",
		ConnectorInstance: a.ID, Complete: true, ObservationCount: 1, ErrorCount: 0, Body: body,
	})
}

// overrideHeuristic is a stable pseudo-random selector so drift is deterministic.
func overrideHeuristic(adapterID, capKey string) bool {
	h := 0
	for _, r := range adapterID + "|" + capKey {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return h%5 == 0
}

func platformFor(enf model.Enforcement) string {
	switch enf {
	case model.EnforcementServerSide:
		return "github"
	case model.EnforcementMDMLocked:
		return "claude_code"
	default:
		return "cursor"
	}
}

func emitDriftObs(st *store.Store, a model.Adapter, c model.Capability, state model.EnvState, ts time.Time) {
	obs := model.Obs("state", c.Key+"_drift", 0, "bool", "amber", "warning", c.Plane, a.ID, c.Disposition,
		map[string]any{"source": state.Source, "enforcement": string(c.Enforcement)}, "env_managed", "")
	body := model.BuildBatch(a, ts, time.Minute, []any{obs}, nil)
	st.AddObservation(model.ObservationRecord{
		ReceivedAt: ts, Contract: model.ObservationContract, ConnectorType: "ai-vendor",
		ConnectorInstance: a.ID, Complete: true, ObservationCount: 1, ErrorCount: 0, Body: body,
	})
}
