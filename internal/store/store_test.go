package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

func TestNew_SeedsCatalog(t *testing.T) {
	s := New()
	if got := len(s.ListAdapters()); got < 16 {
		t.Fatalf("expected >=16 seeded adapters, got %d", got)
	}
	if _, ok := s.GetAdapter("openai"); !ok {
		t.Error("expected openai adapter seeded")
	}
}

func TestPatchAdapter(t *testing.T) {
	s := New()
	proxy := model.ModeProxy
	a, err := s.PatchAdapter("openai", model.AdapterPatch{Mode: &proxy})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if a.Mode != model.ModeProxy {
		t.Errorf("mode not applied: %s", a.Mode)
	}
	bad := model.Mode("nonsense")
	if _, err := s.PatchAdapter("openai", model.AdapterPatch{Mode: &bad}); err != ErrInvalid {
		t.Errorf("expected ErrInvalid for bad mode, got %v", err)
	}
	if _, err := s.PatchAdapter("ghost", model.AdapterPatch{}); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRingBufferEviction(t *testing.T) {
	s := New()
	for i := 0; i < ringMax+25; i++ {
		s.RecordCall(model.CallRecord{AdapterID: "openai", Method: "GET", Path: "/x"})
	}
	if got := len(s.ListCalls("openai", ringMax+100)); got != ringMax {
		t.Errorf("expected calls capped at %d, got %d", ringMax, got)
	}
}

func TestCredentials(t *testing.T) {
	s := New()
	c := s.AddCredential(model.Credential{Name: "vault-openai", SecretRef: "vault://kv/openai"})
	if c.ID == "" {
		t.Error("expected generated credential id")
	}
	if len(s.ListCredentials()) != 1 {
		t.Error("expected one credential")
	}
}

func TestObservationsAndDrift(t *testing.T) {
	s := New()
	s.AddObservation(model.ObservationRecord{ConnectorInstance: "openai", Contract: model.ObservationContract})
	if len(s.ListObservations(10)) != 1 {
		t.Error("expected one observation")
	}
	s.ReplaceDrift([]model.DriftRecord{{Vendor: "openai", Capability: "mcp_allow_deny"}})
	if len(s.ListDrift()) != 1 {
		t.Error("expected one drift record")
	}
}

// A spine-key holder can mint gateway IDs at will; the heartbeat map must not
// grow without bound, and the real gateway's stable ID must never be locked
// out by a flood of forged ones.
func TestSetGatewayEnforcementEvictsOldestWhenFull(t *testing.T) {
	s := New()
	base := time.Now().UTC().Add(-time.Hour)
	s.SetGatewayEnforcement(model.EnforcementReport{GatewayID: "gw@real", BaseURL: "http://127.0.0.1:8125", At: base})
	for i := 0; i < maxGatewayEnforce*2; i++ {
		s.SetGatewayEnforcement(model.EnforcementReport{
			GatewayID: fmt.Sprintf("forged-%d", i), BaseURL: "http://attacker:9",
			At: base.Add(time.Duration(i+1) * time.Second),
		})
	}
	// The real gateway keeps updating in place, so it always has a slot.
	s.SetGatewayEnforcement(model.EnforcementReport{GatewayID: "gw@real", BaseURL: "http://127.0.0.1:8125"})
	reps := s.ListGatewayEnforcement()
	if len(reps) != maxGatewayEnforce {
		t.Fatalf("heartbeat map holds %d, want %d", len(reps), maxGatewayEnforce)
	}
	found := false
	for _, r := range reps {
		if r.GatewayID == "gw@real" {
			found = true
		}
	}
	if !found {
		t.Fatal("real gateway evicted by forged heartbeats")
	}
}
