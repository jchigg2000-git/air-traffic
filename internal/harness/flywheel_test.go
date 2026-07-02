package harness

// Config-shaped proposals (G6-full slice): deny-list fallback, probe-driven
// threshold evidence, manual supersession, and the approve-time validation
// that guards the pack. The full propose→approve→hot-reload loop lives in
// runner_test.go; these exercise the flywheel synthesis rules directly.

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"air-traffic/internal/model"
	"air-traffic/internal/store"
)

func newTestRunner(t *testing.T, presidioURL string) *Runner {
	t.Helper()
	r, err := NewRunner(store.New(), slog.New(slog.NewTextHandler(io.Discard, nil)),
		t.TempDir(), "gwk-test", presidioURL)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func nameMissResults(value string) []model.HarnessResult {
	content := "Write a referral letter for " + value + " living at the clinic."
	return []model.HarnessResult{{
		Content: content,
		Truth: []model.TruthSpan{{
			Start: 28, End: 28 + len(value), Type: "PERSON_NAME", Value: value,
		}},
		MissedTypes: []string{"PERSON_NAME"},
	}}
}

func proposalByID(r *Runner, id string) *model.PatternProposal {
	for _, p := range r.Proposals() {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

func TestFlywheelDenyListFallbackWithoutProbe(t *testing.T) {
	r := newTestRunner(t, "") // no probe: deny-list is the only free-text artifact
	run := &model.HarnessRun{ID: "run1", DetectorChain: "regex,presidio"}

	if promoted := r.flywheel(run, nameMissResults("Diego Mensah")); promoted != 1 {
		t.Errorf("promoted = %d, want 1", promoted)
	}

	p := proposalByID(r, "deny-PERSON_NAME")
	if p == nil || p.Status != "proposed" || p.Kind != model.KindDenyList {
		t.Fatalf("deny-PERSON_NAME = %+v, want proposed deny_list", p)
	}
	if len(p.DenyList) != 1 || p.DenyList[0] != "Diego Mensah" {
		t.Errorf("deny list = %v", p.DenyList)
	}
	if proposalByID(r, "manual-PERSON_NAME") != nil {
		t.Error("manual row should not exist when a deny artifact covers the type")
	}

	pack, err := r.ApproveProposal("deny-PERSON_NAME")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != 1 || len(pack.Rules) != 1 || pack.Rules[0].Kind != model.KindDenyList {
		t.Errorf("pack = %+v", pack)
	}
}

func TestFlywheelStaysManualWithoutPresidioInChain(t *testing.T) {
	r := newTestRunner(t, "")
	run := &model.HarnessRun{ID: "run1", DetectorChain: "regex"}

	r.flywheel(run, nameMissResults("Diego Mensah"))

	if proposalByID(r, "deny-PERSON_NAME") != nil {
		t.Error("deny proposal without presidio in the chain would be a lie")
	}
	p := proposalByID(r, "manual-PERSON_NAME")
	if p == nil || p.Status != "manual" {
		t.Fatalf("manual-PERSON_NAME = %+v", p)
	}
	if _, err := r.ApproveProposal("manual-PERSON_NAME"); err == nil {
		t.Error("manual proposals must stay unapprovable")
	}
}

func TestFlywheelThresholdFromProbeSupersedesManual(t *testing.T) {
	content := "call 5551234567 now"
	var gotPayload map[string]any
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(b, &gotPayload)
		// Presidio saw the phone but scored it under the 0.40 gate.
		_, _ = io.WriteString(w, `[{"entity_type":"PHONE_NUMBER","start":5,"end":15,"score":0.30}]`)
	}))
	defer stub.Close()

	r := newTestRunner(t, stub.URL)
	results := []model.HarnessResult{{
		Content:     content,
		Truth:       []model.TruthSpan{{Start: 5, End: 15, Type: "PHONE", Value: "5551234567"}},
		MissedTypes: []string{"PHONE"},
	}}

	// First run without presidio in the chain: honest limit is manual.
	r.flywheel(&model.HarnessRun{ID: "runA", DetectorChain: "regex"}, results)
	if p := proposalByID(r, "manual-PHONE"); p == nil || p.Status != "manual" {
		t.Fatalf("manual-PHONE = %+v", p)
	}

	// Presidio joins the chain: the probe proves a lower gate catches it.
	r.flywheel(&model.HarnessRun{ID: "runB", DetectorChain: "regex,presidio"}, results)
	if th, _ := gotPayload["score_threshold"].(float64); th != 0 {
		t.Errorf("probe score_threshold = %v, want 0 (raw evidence)", gotPayload["score_threshold"])
	}
	if _, ok := gotPayload["ad_hoc_recognizers"]; ok {
		t.Error("probe must not carry pack recognizers — raw engine scores only")
	}
	p := proposalByID(r, "threshold-PHONE")
	if p == nil || p.Status != "proposed" || p.Kind != model.KindThreshold {
		t.Fatalf("threshold-PHONE = %+v", p)
	}
	if p.Threshold != 0.25 {
		t.Errorf("threshold = %.2f, want 0.25 (one grid step under the 0.30 evidence)", p.Threshold)
	}
	if !strings.Contains(p.Rationale, "0.30") {
		t.Errorf("rationale should carry the probed score: %q", p.Rationale)
	}
	if m := proposalByID(r, "manual-PHONE"); m == nil || m.Status != "superseded" {
		t.Errorf("manual-PHONE = %+v, want superseded", m)
	}

	pack, err := r.ApproveProposal("threshold-PHONE")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Rules[0].Kind != model.KindThreshold || pack.Rules[0].Threshold != 0.25 {
		t.Errorf("rule = %+v", pack.Rules[0])
	}
}

func TestFlywheelDenyTermsDoNotResurrectAfterReject(t *testing.T) {
	r := newTestRunner(t, "")
	run := func(id string) *model.HarnessRun {
		return &model.HarnessRun{ID: id, DetectorChain: "regex,presidio"}
	}

	r.flywheel(run("r1"), nameMissResults("Diego Mensah"))
	if err := r.RejectProposal("deny-PERSON_NAME"); err != nil {
		t.Fatal(err)
	}

	// Same term again: rejected terms stay rejected, no new proposal.
	r.flywheel(run("r2"), nameMissResults("Diego Mensah"))
	for _, p := range r.Proposals() {
		if p.Kind == model.KindDenyList && p.Status == "proposed" {
			t.Fatalf("rejected term resurrected: %+v", p)
		}
	}

	// A genuinely new term opens a fresh row (base ID is settled).
	r.flywheel(run("r3"), nameMissResults("Kofi Novak"))
	p := proposalByID(r, "deny-PERSON_NAME-2")
	if p == nil || p.Status != "proposed" {
		t.Fatalf("deny-PERSON_NAME-2 = %+v", p)
	}
	if len(p.DenyList) != 1 || p.DenyList[0] != "Kofi Novak" {
		t.Errorf("deny list = %v, want only the new term", p.DenyList)
	}
}

func TestRuleFromProposalValidation(t *testing.T) {
	cases := []struct {
		name string
		prop model.PatternProposal
		ok   bool
	}{
		{"regex ok", model.PatternProposal{ID: "a", Type: "SSN", Kind: model.KindRegex, Regex: `\d{9}`}, true},
		{"legacy empty kind is regex", model.PatternProposal{ID: "a", Type: "SSN", Regex: `\d{9}`}, true},
		{"bad regex", model.PatternProposal{ID: "a", Type: "SSN", Kind: model.KindRegex, Regex: `(`}, false},
		{"deny ok", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindDenyList, DenyList: []string{"Diego Mensah"}}, true},
		{"deny empty", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindDenyList}, false},
		{"deny metachars", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindDenyList, DenyList: []string{"Diego.*"}}, false},
		{"threshold ok", model.PatternProposal{ID: "a", Type: "PHONE", Kind: model.KindThreshold, Threshold: 0.25}, true},
		{"threshold too low", model.PatternProposal{ID: "a", Type: "PHONE", Kind: model.KindThreshold, Threshold: 0.01}, false},
		{"unknown kind", model.PatternProposal{ID: "a", Type: "PHONE", Kind: "banana"}, false},
	}
	for _, c := range cases {
		_, err := ruleFromProposal(c.prop)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestProposeGateGrid(t *testing.T) {
	for _, c := range []struct{ score, want float64 }{
		{0.30, 0.25}, {0.39, 0.30}, {0.10, 0.05}, {0.05, 0.05}, {0.01, 0.05},
	} {
		if got := proposeGate(c.score); got != c.want {
			t.Errorf("proposeGate(%.2f) = %.2f, want %.2f", c.score, got, c.want)
		}
	}
}

// Guard against the ordering trap fixed alongside this feature: an append
// mid-upsert must not strand updates on a stale backing array.
func TestUpsertSurvivesSliceGrowth(t *testing.T) {
	r := newTestRunner(t, "")
	run := &model.HarnessRun{ID: "r1", DetectorChain: "regex,presidio"}

	// Round 1: several types at once forces interleaved appends.
	results := nameMissResults("Diego Mensah")
	results = append(results, model.HarnessResult{
		Content:     "member SSN on file: 123456789.",
		Truth:       []model.TruthSpan{{Start: 20, End: 29, Type: "SSN", Value: "123456789"}},
		MissedTypes: []string{"SSN"},
	})
	r.flywheel(run, results)

	// Round 2: the same misses again must increment, not vanish.
	r.flywheel(&model.HarnessRun{ID: "r2", DetectorChain: "regex,presidio"}, results)

	ssn := proposalByID(r, "ssn-bare-context")
	if ssn == nil || ssn.SampleMisses != 2 {
		t.Fatalf("ssn-bare-context = %+v, want 2 accumulated misses", ssn)
	}
	deny := proposalByID(r, "deny-PERSON_NAME")
	if deny == nil || deny.SampleMisses != 2 {
		t.Fatalf("deny-PERSON_NAME = %+v, want 2 accumulated misses", deny)
	}
}
