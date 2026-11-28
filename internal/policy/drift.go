package policy

import (
	"sort"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/envconfig"
	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
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
// all is a warning (the label is aspirational); and a fresh gateway still
// enforcing a pulled baseline while the control plane has no policy applied is
// a warning (split brain, PIVOT-13 — the two halves disagree about what is in
// force, and a restart of that gateway would drop to a third posture).
func gatewayDrift(st *store.Store, ts time.Time) []model.DriftRecord {
	expected := false
	pol := st.GetPolicy()
	if pol != nil {
		if b, ok := BaselineByID(pol.Baseline); ok {
			expected = b.PIIRedaction != "off"
		}
	}
	var splitBrain []model.EnforcementReport
	if pol == nil {
		splitBrain = enforcingWithoutPolicy(st)
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
			_, fresh, everSeen := st.GatewayEnforcement(a.ID, c.Key, model.GatewayStaleAfter)
			if fresh {
				if rep, ok := firstListing(splitBrain, a.ID, c.Key); ok {
					records = append(records, model.DriftRecord{
						Vendor: a.ID, Capability: c.Key, Plane: c.Plane, Surface: c.Disposition,
						Declared: "no policy applied", Actual: "gateway enforcing baseline " + rep.PolicyBaseline + " (action " + rep.Action + ")",
						Severity: "warning",
						Message: "control plane has no applied policy but gateway " + rep.GatewayID + " is still enforcing " + c.Key +
							" under previously pulled baseline " + rep.PolicyBaseline + "; a restart of that gateway would fall back to its default action",
						DetectedAt: ts,
					})
					emitGatewayDriftObs(st, a, c, "amber", "warning", ts)
				}
				continue
			}
			switch {
			case everSeen:
				records = append(records, model.DriftRecord{
					Vendor: a.ID, Capability: c.Key, Plane: c.Plane, Surface: c.Disposition,
					Declared: "proxy_enforced", Actual: "unenforced (gateway stale)", Severity: "error",
					Message:    "gateway enforcement heartbeat for " + c.Key + " went stale; requests are no longer filtered",
					DetectedAt: ts,
				})
				emitGatewayDriftObs(st, a, c, "red", "error", ts)
			case expected && c.Key == "pii_redaction":
				records = append(records, model.DriftRecord{
					Vendor: a.ID, Capability: c.Key, Plane: c.Plane, Surface: c.Disposition,
					Declared: "proxy_enforced (policy intent)", Actual: "no gateway reporting", Severity: "warning",
					Message:    "applied baseline expects PII redaction but no gateway has reported enforcement",
					DetectedAt: ts,
				})
				emitGatewayDriftObs(st, a, c, "amber", "warning", ts)
			}
		}
	}
	return records
}

// enforcingWithoutPolicy returns the fresh heartbeats that still carry a
// pulled policy baseline, sorted by gateway id so drift output is stable. An
// empty PolicyBaseline means the gateway has never applied a pulled policy or
// its action is pinned by GATEWAY_REDACT_ACTION — neither disagrees with a
// control plane that has no policy. Freshness uses wall-clock time, matching
// store.GatewayEnforcement, because heartbeats are stamped in real time.
func enforcingWithoutPolicy(st *store.Store) []model.EnforcementReport {
	cutoff := time.Now().UTC().Add(-model.GatewayStaleAfter)
	var out []model.EnforcementReport
	for _, rep := range st.ListGatewayEnforcement() {
		if rep.PolicyBaseline != "" && rep.At.After(cutoff) {
			out = append(out, rep)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GatewayID < out[j].GatewayID })
	return out
}

// firstListing returns the first report whose vendor list names capability.
func firstListing(reps []model.EnforcementReport, vendor, capability string) (model.EnforcementReport, bool) {
	for _, rep := range reps {
		for _, k := range rep.Vendors[vendor] {
			if k == capability {
				return rep, true
			}
		}
	}
	return model.EnforcementReport{}, false
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
