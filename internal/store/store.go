// Package store is the in-memory state for Air-Traffic (mirrors it-scorecard store):
// adapters, credentials, recorded calls, observations, audit + drift, current policy.
package store

import (
	"errors"
	"sort"
	"sync"
	"time"

	"air-traffic/internal/catalog"
	"air-traffic/internal/model"
)

const ringMax = 5000

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
)

// Store holds all mutable runtime state behind a single RWMutex.
type Store struct {
	mu           sync.RWMutex
	adapters     map[string]model.Adapter
	order        []string
	credentials  map[string]model.Credential
	calls        []model.CallRecord
	observations []model.ObservationRecord
	audit        []model.AuditEvent
	activity     []model.ActivityEvent
	drift        []model.DriftRecord

	// The applied policy, write-through to disk when policyFile is set
	// (policy_persist.go) — it is enforcement intent the gateway is already
	// acting on, so losing it to a restart makes the two halves disagree.
	policy           *model.Policy
	policyFile       policyPersister
	policyPersistErr error

	inferCaptures []model.InferenceCapture
	gwReports     []model.GatewayRequestReport
	gwEnforce     map[string]model.EnforcementReport
	patternPack   model.PatternPack

	// Keystore (keystore.go). Unlike the rings above this is write-through to
	// disk when keyFile is set — credentials must survive a restart.
	apps           map[string]model.App
	keys           map[string]model.APIKey
	keysVersion    int
	keysUpdatedAt  time.Time
	keyFile        keyPersister
	keysPersistErr error

	nextCallID  int64
	nextObsID   int64
	nextEventID int64
	nextDriftID int64
	nextInferID int64
}

// New builds a store seeded from the vendor catalog.
func New() *Store {
	s := &Store{
		adapters:    make(map[string]model.Adapter),
		credentials: make(map[string]model.Credential),
		gwEnforce:   make(map[string]model.EnforcementReport),
		apps:        make(map[string]model.App),
		keys:        make(map[string]model.APIKey),
		nextCallID:  1, nextObsID: 1, nextEventID: 1, nextDriftID: 1, nextInferID: 1,
	}
	s.seed()
	return s
}

// defaultRoster is the set of vendors enabled at boot. Justin's choice: the six
// Tier-1 deep adapters are on; the other ten ship disabled until their per-vendor
// auth config is built (docs/plans/TODO-vendor-auth.md). Do NOT auto-toggle these.
var defaultRoster = map[string]bool{
	"openai": true, "anthropic": true, "bedrock": true,
	"azure_openai": true, "vertex": true, "github_copilot": true,
}

func (s *Store) seed() {
	now := time.Now().UTC()
	for _, d := range catalog.All() {
		a := model.Adapter{
			ID:           d.ID,
			Vendor:       d.Vendor,
			DisplayName:  d.DisplayName,
			Family:       d.Family,
			APIVersion:   d.APIVersion,
			Tier:         d.Tier,
			Mode:         model.ModeSynthetic,
			Enabled:      defaultRoster[d.ID],
			Emit:         true,
			BasePath:     "/synthetic/" + d.ID,
			Scenario:     "healthy",
			BAASigned:    d.BAASigned,
			Capabilities: d.Capabilities,
			Status:       model.Status{State: "unknown", Message: "Not tested yet", CheckedAt: now},
			UpdatedAt:    now,
		}
		s.adapters[d.ID] = a
		s.order = append(s.order, d.ID)
	}
	s.recordActivity("system", "Seeded vendor adapter catalog", "")
}

// ---- adapters ----

func (s *Store) ListAdapters() []model.Adapter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Adapter, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.adapters[id])
	}
	return out
}

func (s *Store) GetAdapter(id string) (model.Adapter, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.adapters[id]
	return a, ok
}

func (s *Store) PatchAdapter(id string, p model.AdapterPatch) (model.Adapter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.adapters[id]
	if !ok {
		return model.Adapter{}, ErrNotFound
	}
	if p.Mode != nil {
		switch *p.Mode {
		case model.ModeDisabled, model.ModeSynthetic, model.ModeProxy:
			a.Mode = *p.Mode
		default:
			return model.Adapter{}, ErrInvalid
		}
	}
	if p.Enabled != nil {
		a.Enabled = *p.Enabled
	}
	if p.Emit != nil {
		a.Emit = *p.Emit
	}
	if p.Scenario != nil {
		a.Scenario = *p.Scenario
	}
	if p.UpstreamURL != nil {
		a.UpstreamURL = *p.UpstreamURL
	}
	if p.EndpointConfig != nil {
		a.EndpointConfig = *p.EndpointConfig
	}
	a.UpdatedAt = time.Now().UTC()
	s.adapters[id] = a
	s.recordActivity("adapter", "Updated adapter "+id, "")
	return a, nil
}

// SetScenario is a convenience used by the synthetic harness control path.
func (s *Store) SetScenario(id, scenario string) (model.Adapter, error) {
	return s.PatchAdapter(id, model.AdapterPatch{Scenario: &scenario})
}

func (s *Store) SetAdapterStatus(id string, st model.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.adapters[id]; ok {
		a.Status = st
		a.UpdatedAt = time.Now().UTC()
		s.adapters[id] = a
	}
}

// ---- calls ----

func (s *Store) RecordCall(c model.CallRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = s.nextCallID
	s.nextCallID++
	c.RecordedAt = time.Now().UTC()
	s.calls = append(s.calls, c)
	if len(s.calls) > ringMax {
		s.calls = s.calls[len(s.calls)-ringMax:]
	}
}

func (s *Store) ListCalls(adapterID string, limit int) []model.CallRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.CallRecord, 0, limit)
	for i := len(s.calls) - 1; i >= 0 && len(out) < limit; i-- {
		if adapterID == "" || s.calls[i].AdapterID == adapterID {
			out = append(out, s.calls[i])
		}
	}
	return out
}

func (s *Store) ResetCalls(adapterID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.calls[:0]
	for _, c := range s.calls {
		if c.AdapterID != adapterID {
			kept = append(kept, c)
		}
	}
	s.calls = kept
}

// ---- observations ----

func (s *Store) AddObservation(r model.ObservationRecord) model.ObservationRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = s.nextObsID
	s.nextObsID++
	if r.ReceivedAt.IsZero() {
		r.ReceivedAt = time.Now().UTC()
	}
	s.observations = append(s.observations, r)
	if len(s.observations) > ringMax {
		s.observations = s.observations[len(s.observations)-ringMax:]
	}
	return r
}

func (s *Store) ListObservations(limit int) []model.ObservationRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.ObservationRecord, 0, limit)
	for i := len(s.observations) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.observations[i])
	}
	return out
}

// ---- audit ----

func (s *Store) AddAudit(e model.AuditEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.ID == "" {
		e.ID = model.NewUUID()
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	s.audit = append(s.audit, e)
	if len(s.audit) > ringMax {
		s.audit = s.audit[len(s.audit)-ringMax:]
	}
}

func (s *Store) ListAudit(limit int) []model.AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.AuditEvent, 0, limit)
	for i := len(s.audit) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.audit[i])
	}
	return out
}

// ---- drift ----

func (s *Store) ReplaceDrift(records []model.DriftRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range records {
		records[i].ID = s.nextDriftID
		s.nextDriftID++
		if records[i].DetectedAt.IsZero() {
			records[i].DetectedAt = time.Now().UTC()
		}
	}
	s.drift = records
}

func (s *Store) ListDrift() []model.DriftRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.DriftRecord, len(s.drift))
	copy(out, s.drift)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Vendor < out[j].Vendor })
	return out
}

// ---- policy ----

// SetPolicy applies a policy and, when persistence is enabled, writes it
// through to disk.
//
// A persist failure does not fail the apply: the policy IS applied in memory,
// the gateway will pull it within one interval, and refusing the apply would
// mean a full disk stops enforcement changes. The error is recorded instead
// and reported on the gateway-status route, because the failure mode it
// creates — an applied policy that will not survive a restart — is invisible
// until the restart.
func (s *Store) SetPolicy(p model.Policy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.AppliedAt = time.Now().UTC()
	s.policy = &p
	if s.policyFile != nil {
		s.policyPersistErr = s.policyFile.save(p)
	}
}

func (s *Store) GetPolicy() *model.Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

// ---- credentials ----

func (s *Store) AddCredential(c model.Credential) model.Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ID == "" {
		c.ID = model.NewUUID()
	}
	now := time.Now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now
	if c.Status.State == "" {
		c.Status = model.Status{State: "unknown", Message: "not tested", CheckedAt: now}
	}
	s.credentials[c.ID] = c
	return c
}

func (s *Store) ListCredentials() []model.Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Credential, 0, len(s.credentials))
	for _, c := range s.credentials {
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// ---- activity ----

func (s *Store) recordActivity(source, msg, detail string) {
	// caller must hold the lock OR be in New() before goroutines start.
	s.activity = append(s.activity, model.ActivityEvent{
		ID: s.nextEventID, Source: source, Message: msg, Detail: detail, CreatedAt: time.Now().UTC(),
	})
	s.nextEventID++
	if len(s.activity) > ringMax {
		s.activity = s.activity[len(s.activity)-ringMax:]
	}
}

func (s *Store) RecordActivity(source, msg, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordActivity(source, msg, detail)
}

func (s *Store) ListActivity(limit int) []model.ActivityEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.ActivityEvent, 0, limit)
	for i := len(s.activity) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.activity[i])
	}
	return out
}
