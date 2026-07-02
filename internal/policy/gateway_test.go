package policy

import (
	"testing"
	"time"

	"air-traffic/internal/model"
	"air-traffic/internal/store"
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
