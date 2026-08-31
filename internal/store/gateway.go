package store

import (
	"time"

	"air-traffic/internal/model"
)

// ---- inference captures (synthetic mock upstream) ----

func (s *Store) RecordInferenceCapture(c model.InferenceCapture) model.InferenceCapture {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = s.nextInferID
	s.nextInferID++
	s.inferCaptures = append(s.inferCaptures, c)
	if len(s.inferCaptures) > ringMax {
		s.inferCaptures = s.inferCaptures[len(s.inferCaptures)-ringMax:]
	}
	return c
}

func (s *Store) ListInferenceCaptures(adapterID string, limit int) []model.InferenceCapture {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.InferenceCapture, 0, limit)
	for i := len(s.inferCaptures) - 1; i >= 0 && len(out) < limit; i-- {
		if adapterID == "" || s.inferCaptures[i].AdapterID == adapterID {
			out = append(out, s.inferCaptures[i])
		}
	}
	return out
}

// InferenceCaptureByRequestID joins a capture back to the gateway request that
// produced it (the harness scoring join).
func (s *Store) InferenceCaptureByRequestID(gatewayRequestID string) (model.InferenceCapture, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.inferCaptures) - 1; i >= 0; i-- {
		if s.inferCaptures[i].GatewayRequestID == gatewayRequestID {
			return s.inferCaptures[i], true
		}
	}
	return model.InferenceCapture{}, false
}

// ---- gateway request reports (per-request redaction audits) ----

func (s *Store) AddGatewayReports(reports []model.GatewayRequestReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gwReports = append(s.gwReports, reports...)
	if len(s.gwReports) > ringMax {
		s.gwReports = s.gwReports[len(s.gwReports)-ringMax:]
	}
}

func (s *Store) GatewayReportByRequestID(requestID string) (model.GatewayRequestReport, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.gwReports) - 1; i >= 0; i-- {
		if s.gwReports[i].RequestID == requestID {
			return s.gwReports[i], true
		}
	}
	return model.GatewayRequestReport{}, false
}

func (s *Store) ListGatewayReports(limit int) []model.GatewayRequestReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.GatewayRequestReport, 0, limit)
	for i := len(s.gwReports) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.gwReports[i])
	}
	return out
}

// ---- gateway enforcement heartbeats ----

func (s *Store) SetGatewayEnforcement(r model.EnforcementReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	s.gwEnforce[r.GatewayID] = r
}

// GatewayEnforcement reports whether any gateway is (or ever was) enforcing
// the capability: fresh means a heartbeat younger than staleAfter listed it.
func (s *Store) GatewayEnforcement(vendor, capability string, staleAfter time.Duration) (gatewayID string, fresh, everSeen bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cutoff := time.Now().UTC().Add(-staleAfter)
	for id, rep := range s.gwEnforce {
		for _, capKey := range rep.Vendors[vendor] {
			if capKey != capability {
				continue
			}
			everSeen = true
			if rep.At.After(cutoff) {
				return id, true, true
			}
			gatewayID = id
		}
	}
	return gatewayID, false, everSeen
}

func (s *Store) ListGatewayEnforcement() []model.EnforcementReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.EnforcementReport, 0, len(s.gwEnforce))
	for _, r := range s.gwEnforce {
		out = append(out, r)
	}
	return out
}

// ---- pattern pack (flywheel-managed detector additions) ----

func (s *Store) GetPatternPack() model.PatternPack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.patternPack.Rules == nil {
		return model.PatternPack{Version: s.patternPack.Version, Rules: []model.PatternRule{}}
	}
	return s.patternPack
}

func (s *Store) SetPatternPack(p model.PatternPack) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patternPack = p
}
