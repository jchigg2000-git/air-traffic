package policy

import (
	"testing"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
)

func totalCaps(st *store.Store) int {
	n := 0
	for _, a := range st.ListAdapters() {
		n += len(a.Capabilities)
	}
	return n
}

func TestBaselines(t *testing.T) {
	bs := Baselines()
	if len(bs) != 4 {
		t.Fatalf("expected 4 baselines, got %d", len(bs))
	}
	if _, ok := BaselineByID("healthcare"); !ok {
		t.Error("expected healthcare baseline")
	}
}

func TestApply_CoverageSumsToCaps(t *testing.T) {
	st := store.New()
	rep := Apply(st, model.Policy{Baseline: "fintech"})
	if len(rep.Rows) != totalCaps(st) {
		t.Errorf("rows %d != total caps %d", len(rep.Rows), totalCaps(st))
	}
	sum := 0
	for _, c := range rep.Summary {
		sum += c
	}
	if sum != len(rep.Rows) {
		t.Errorf("summary sum %d != rows %d", sum, len(rep.Rows))
	}
	if st.GetPolicy() == nil {
		t.Error("policy not persisted")
	}
}

func TestApply_HealthcareExcludesNonBAA(t *testing.T) {
	st := store.New()
	rep := Apply(st, model.Policy{Baseline: "healthcare"})
	// cohere is not BAA-signed → all its rows must be excluded_no_baa.
	var sawCohere bool
	for _, row := range rep.Rows {
		if row.Vendor == "cohere" {
			sawCohere = true
			if row.Outcome != "excluded_no_baa" {
				t.Errorf("cohere cap %s should be excluded_no_baa under healthcare, got %s", row.Capability, row.Outcome)
			}
		}
		if row.Vendor == "bedrock" && row.Outcome == "excluded_no_baa" {
			t.Errorf("bedrock is BAA-signed; should not be excluded")
		}
	}
	if !sawCohere {
		t.Error("expected cohere rows in report")
	}
}

func TestApply_Classification(t *testing.T) {
	st := store.New()
	rep := Apply(st, model.Policy{Baseline: "general_saas"})
	valid := map[string]bool{
		"applied_native": true, "applied_env": true, "proxy_needed": true,
		"monitored": true, "unverified": true, "unsupported": true, "excluded_no_baa": true,
	}
	for _, row := range rep.Rows {
		if !valid[row.Outcome] {
			t.Errorf("invalid outcome %q for %s/%s", row.Outcome, row.Vendor, row.Capability)
		}
	}
}

func TestRefreshDrift_Deterministic(t *testing.T) {
	st := store.New()
	RefreshDrift(st, time.Now().UTC())
	d1 := st.ListDrift()
	if len(d1) == 0 {
		t.Fatal("expected at least one drift record (deterministic heuristic)")
	}
	RefreshDrift(st, time.Now().UTC())
	d2 := st.ListDrift()
	if len(d1) != len(d2) {
		t.Errorf("drift not deterministic: %d then %d", len(d1), len(d2))
	}
	for _, d := range d1 {
		if d.Surface != model.DispEnvManaged {
			t.Errorf("drift should be on env_managed surface, got %s", d.Surface)
		}
	}
}
