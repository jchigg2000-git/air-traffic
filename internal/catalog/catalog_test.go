package catalog

import (
	"testing"

	"air-traffic/internal/model"
)

func validDisposition(d model.Disposition) bool {
	switch d {
	case model.DispVendorNative, model.DispEnvManaged, model.DispProxyEnforced,
		model.DispMonitorOnly, model.DispUnverified, model.DispUnsupported:
		return true
	}
	return false
}

func validEnforcement(e model.Enforcement) bool {
	switch e {
	case model.EnforcementServerSide, model.EnforcementMDMLocked, model.EnforcementSeedOnly:
		return true
	}
	return false
}

func TestAll_BreadthAndUniqueness(t *testing.T) {
	defs := All()
	if len(defs) < 16 {
		t.Fatalf("expected >=16 vendor definitions, got %d", len(defs))
	}
	seen := map[string]bool{}
	for _, d := range defs {
		if d.ID == "" || d.Vendor == "" {
			t.Errorf("definition with empty id/vendor: %+v", d)
		}
		if seen[d.ID] {
			t.Errorf("duplicate adapter id: %s", d.ID)
		}
		seen[d.ID] = true
		if d.Tier < 1 || d.Tier > 3 {
			t.Errorf("%s: tier out of range: %d", d.ID, d.Tier)
		}
		if len(d.Capabilities) == 0 {
			t.Errorf("%s: no capabilities", d.ID)
		}
	}
}

func TestCapabilities_ValidDispositionsAndEnforcement(t *testing.T) {
	for _, d := range All() {
		for _, c := range d.Capabilities {
			if !validDisposition(c.Disposition) {
				t.Errorf("%s/%s: invalid disposition %q", d.ID, c.Key, c.Disposition)
			}
			if c.Disposition == model.DispEnvManaged {
				if !validEnforcement(c.Enforcement) {
					t.Errorf("%s/%s: env_managed cap missing valid enforcement tier (got %q)", d.ID, c.Key, c.Enforcement)
				}
			} else if c.Enforcement != "" {
				t.Errorf("%s/%s: non-env_managed cap should not carry enforcement %q", d.ID, c.Key, c.Enforcement)
			}
		}
	}
}

func TestMetrics_ValidPlanes(t *testing.T) {
	for _, d := range All() {
		for _, m := range d.Metrics {
			switch m.Plane {
			case model.PlaneDeveloperWorkflow, model.PlaneDataPolicy, model.PlaneBudget, model.PlaneObservability:
			default:
				t.Errorf("%s/%s: invalid plane %q", d.ID, m.Key, m.Plane)
			}
			if m.Kind != "metric" && m.Kind != "state" {
				t.Errorf("%s/%s: invalid kind %q", d.ID, m.Key, m.Kind)
			}
		}
	}
}

func TestByID(t *testing.T) {
	if _, ok := ByID("openai"); !ok {
		t.Error("expected openai definition")
	}
	if _, ok := ByID("nope"); ok {
		t.Error("did not expect unknown definition")
	}
}
