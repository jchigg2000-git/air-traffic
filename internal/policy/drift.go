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
	st.ReplaceDrift(records)
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
