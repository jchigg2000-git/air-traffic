package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

// The defect this closes: SetPolicy assigned a pointer and nothing else, so a
// restart discarded the applied baseline while the gateway kept enforcing the
// action it had already pulled.
func TestPolicySurvivesRestart(t *testing.T) {
	dir := t.TempDir()

	first := New()
	if err := first.EnablePolicyPersistence(dir); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if first.GetPolicy() != nil {
		t.Fatal("first boot must start with no policy applied")
	}
	first.SetPolicy(model.Policy{
		Baseline: "healthcare",
		Vendors:  map[string]map[string]any{model.ZDRAttestationVendor: {model.ZDRAttestationKey: true}},
	})
	if err := first.PolicyPersistError(); err != nil {
		t.Fatalf("write-through failed: %v", err)
	}

	// A whole new process would build a fresh store from the same directory.
	second := New()
	if err := second.EnablePolicyPersistence(dir); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := second.GetPolicy()
	if got == nil {
		t.Fatal("applied policy did not survive the restart")
	}
	if got.Baseline != "healthcare" {
		t.Errorf("baseline = %q, want healthcare", got.Baseline)
	}
	// The attestation has to survive too, or a restart silently turns a masking
	// healthcare posture back into a total block.
	if !model.ZDRAttestedIn(got) {
		t.Error("ZDR attestation did not survive the restart")
	}
	if got.AppliedAt.IsZero() {
		t.Error("applied_at lost in the round trip")
	}
}

// Unlike the keystore, a corrupt policy file must not stop the control plane
// from booting: the operator needs the Rigor Console to re-apply one.
func TestCorruptPolicyIsReportedNotFatal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, policyFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := New()
	err := st.EnablePolicyPersistence(dir)
	if err == nil {
		t.Fatal("a corrupt policy file must be reported to the caller")
	}
	if st.GetPolicy() != nil {
		t.Error("a corrupt file must leave no policy applied, not a partial one")
	}
	// Boot continues, and a fresh apply repairs the file rather than inheriting
	// the corruption.
	st.SetPolicy(model.Policy{Baseline: "fintech"})
	if err := st.PolicyPersistError(); err != nil {
		t.Fatalf("apply after corrupt load: %v", err)
	}
	reloaded := New()
	if err := reloaded.EnablePolicyPersistence(dir); err != nil {
		t.Fatalf("reload after repair: %v", err)
	}
	if p := reloaded.GetPolicy(); p == nil || p.Baseline != "fintech" {
		t.Errorf("repaired policy = %+v, want fintech", p)
	}
}

// Without EnablePolicyPersistence the store stays purely in-memory — the bare
// `go test`/library posture, and the reason SetPolicy must tolerate a nil file.
func TestSetPolicyWithoutPersistenceIsInMemoryOnly(t *testing.T) {
	st := New()
	st.SetPolicy(model.Policy{Baseline: "fintech"})
	if p := st.GetPolicy(); p == nil || p.Baseline != "fintech" {
		t.Fatalf("in-memory policy = %+v", p)
	}
	if err := st.PolicyPersistError(); err != nil {
		t.Errorf("no persistence configured should report no error, got %v", err)
	}
}
