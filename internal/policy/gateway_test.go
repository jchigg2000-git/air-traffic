package policy

import (
	"testing"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
)

func pickRow(rep model.CoverageReport, vendor, capability string) (model.CoverageRow, bool) {
	for _, row := range rep.Rows {
		if row.Vendor == vendor && row.Capability == capability {
			return row, true
		}
	}
	return model.CoverageRow{}, false
}

// The honesty flip: proxy_enforced classifies as applied_proxy only while a
// fresh heartbeat lists the capability; otherwise it stays proxy_needed.
func TestApplyFlipsProxyEnforcedOnFreshHeartbeat(t *testing.T) {
	st := store.New()

	rep := Apply(st, model.Policy{Baseline: "healthcare"})
	row, ok := pickRow(rep, "anthropic", "pii_redaction")
	if !ok {
		t.Fatal("anthropic pii_redaction row missing")
	}
	if row.Outcome != "proxy_needed" {
		t.Errorf("without heartbeat outcome = %q, want proxy_needed", row.Outcome)
	}

	st.SetGatewayEnforcement(model.EnforcementReport{
		GatewayID: "gw@test", Vendors: map[string][]string{"anthropic": {"pii_redaction"}},
	})
	rep = Apply(st, model.Policy{Baseline: "healthcare"})
	row, _ = pickRow(rep, "anthropic", "pii_redaction")
	if row.Outcome != "applied_proxy" || row.Signal.Code != "success" {
		t.Errorf("with heartbeat outcome = %q signal %+v, want applied_proxy/success", row.Outcome, row.Signal)
	}

	// Other proxy_enforced capabilities the gateway does NOT enforce stay honest.
	row, _ = pickRow(rep, "anthropic", "hard_spend_cap")
	if row.Outcome != "proxy_needed" {
		t.Errorf("hard_spend_cap outcome = %q, want proxy_needed (gateway never claimed it)", row.Outcome)
	}
}

func TestGatewayDriftOnStaleHeartbeat(t *testing.T) {
	st := store.New()
	Apply(st, model.Policy{Baseline: "healthcare"})

	// Enforcement seen, then gone stale.
	st.SetGatewayEnforcement(model.EnforcementReport{
		GatewayID: "gw@test", Vendors: map[string][]string{"anthropic": {"pii_redaction"}},
		At: time.Now().UTC().Add(-10 * time.Minute),
	})
	RefreshDrift(st, time.Now().UTC())

	foundStale := false
	for _, d := range st.ListDrift() {
		if d.Vendor == "anthropic" && d.Capability == "pii_redaction" && d.Severity == "error" {
			foundStale = true
			if d.Actual != "unenforced (gateway stale)" {
				t.Errorf("actual = %q", d.Actual)
			}
		}
	}
	if !foundStale {
		t.Error("stale gateway heartbeat did not raise error-severity drift")
	}
}

func TestGatewayDriftWarnsWhenPolicyExpectsGateway(t *testing.T) {
	st := store.New()
	Apply(st, model.Policy{Baseline: "healthcare"}) // pii_redaction on+phi, no gateway at all
	RefreshDrift(st, time.Now().UTC())

	found := false
	for _, d := range st.ListDrift() {
		if d.Vendor == "anthropic" && d.Capability == "pii_redaction" && d.Severity == "warning" {
			found = true
		}
	}
	if !found {
		t.Error("policy expecting PII redaction with no gateway should warn")
	}
}

func TestNoGatewayDriftUnderGeneralSaaS(t *testing.T) {
	st := store.New()
	Apply(st, model.Policy{Baseline: "general_saas"}) // pii_redaction off
	RefreshDrift(st, time.Now().UTC())
	for _, d := range st.ListDrift() {
		if d.Capability == "pii_redaction" {
			t.Errorf("general_saas should not raise pii_redaction drift: %+v", d)
		}
	}
}

// splitBrainDrift returns the PIVOT-13 records: the control plane has no policy
// while a gateway still enforces one it pulled earlier.
func splitBrainDrift(st *store.Store) []model.DriftRecord {
	var out []model.DriftRecord
	for _, d := range st.ListDrift() {
		if d.Surface == model.DispProxyEnforced && d.Declared == "no policy applied" {
			out = append(out, d)
		}
	}
	return out
}

func enforcingHeartbeat(baseline string, at time.Time) model.EnforcementReport {
	return model.EnforcementReport{
		GatewayID: "gw@test", Action: "block", PolicyBaseline: baseline, At: at,
		Vendors: map[string][]string{"anthropic": {"pii_redaction"}},
	}
}

func TestGatewayDriftWarnsOnSplitBrain(t *testing.T) {
	st := store.New() // no policy applied
	st.SetGatewayEnforcement(enforcingHeartbeat("healthcare", time.Now().UTC()))
	RefreshDrift(st, time.Now().UTC())

	got := splitBrainDrift(st)
	if len(got) != 1 {
		t.Fatalf("split-brain records = %+v, want exactly one", got)
	}
	d := got[0]
	if d.Vendor != "anthropic" || d.Capability != "pii_redaction" || d.Severity != "warning" {
		t.Errorf("record = %+v, want anthropic/pii_redaction warning", d)
	}
}

func TestNoSplitBrainDriftWhenGatewayHasNoPulledBaseline(t *testing.T) {
	st := store.New()
	// Action pinned by GATEWAY_REDACT_ACTION: the heartbeat carries no baseline.
	st.SetGatewayEnforcement(enforcingHeartbeat("", time.Now().UTC()))
	RefreshDrift(st, time.Now().UTC())
	if got := splitBrainDrift(st); len(got) != 0 {
		t.Errorf("pinned gateway raised split-brain drift: %+v", got)
	}
}

func TestNoSplitBrainDriftWhenPolicyApplied(t *testing.T) {
	st := store.New()
	Apply(st, model.Policy{Baseline: "healthcare"})
	st.SetGatewayEnforcement(enforcingHeartbeat("healthcare", time.Now().UTC()))
	RefreshDrift(st, time.Now().UTC())
	if got := splitBrainDrift(st); len(got) != 0 {
		t.Errorf("applied policy raised split-brain drift: %+v", got)
	}
}

func TestNoSplitBrainDriftOnStaleHeartbeat(t *testing.T) {
	st := store.New()
	st.SetGatewayEnforcement(enforcingHeartbeat("healthcare", time.Now().UTC().Add(-10*time.Minute)))
	RefreshDrift(st, time.Now().UTC())
	if got := splitBrainDrift(st); len(got) != 0 {
		t.Errorf("stale heartbeat raised split-brain drift: %+v", got)
	}
}
