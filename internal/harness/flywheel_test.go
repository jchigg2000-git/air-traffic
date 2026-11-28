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
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
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
		// allow_list is the one kind that switches detection off, so its
		// bounds are the trust boundary.
		{"allow ok", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: []string{"Sherlock Holmes", "Baker Street"}}, true},
		{"allow empty", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList}, false},
		{"allow too many terms", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: nTerms(maxAllowTerms + 1)}, false},
		{"allow max terms", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: nTerms(maxAllowTerms)}, true},
		{"allow padded term", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: []string{" Sherlock"}}, false},
		{"allow blank term", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: []string{"  "}}, false},
		{"allow term at limit", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: []string{strings.Repeat("x", maxAllowTermLen)}}, true},
		{"allow term too long", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: []string{strings.Repeat("x", maxAllowTermLen+1)}}, false},
		{"allow term newline", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: []string{"Sherlock\nHolmes"}}, false},
		{"allow term tab", model.PatternProposal{ID: "a", Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: []string{"Sherlock\tHolmes"}}, false},
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

// nTerms builds n distinct valid allow/deny terms.
func nTerms(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "term" + strconv.Itoa(i)
	}
	return out
}

func TestAddProposalRequiresTypeAndRationale(t *testing.T) {
	r := newTestRunner(t, "")
	valid := model.PatternProposal{Kind: model.KindAllowList, AllowList: []string{"Sherlock Holmes"}}

	noType := valid
	noType.Rationale = "fictional character in a test fixture"
	if _, err := r.AddProposal(noType); err == nil {
		t.Error("empty Type accepted, want error")
	}
	noRationale := valid
	noRationale.Type = "PERSON_NAME"
	if _, err := r.AddProposal(noRationale); err == nil {
		t.Error("empty Rationale accepted, want error")
	}
	if n := len(r.Proposals()); n != 0 {
		t.Errorf("%d proposals recorded after rejected adds, want 0", n)
	}
}

func TestAddProposalStampsManualAndRejectsDuplicate(t *testing.T) {
	r := newTestRunner(t, "")
	got, err := r.AddProposal(model.PatternProposal{
		Type: "PERSON_NAME", Kind: model.KindAllowList, AllowList: []string{"Sherlock Holmes"},
		Rationale: "fictional character", Status: "approved", SourceRun: "run-xyz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "proposed" || got.SourceRun != "manual" {
		t.Errorf("status/source = %q/%q, want proposed/manual (caller values must not stick)", got.Status, got.SourceRun)
	}
	if !regexp.MustCompile(`^manual-person_name-[0-9a-f]{8}$`).MatchString(got.ID) {
		t.Errorf("generated ID = %q, want manual-person_name-<8 hex>", got.ID)
	}
	if p := proposalByID(r, got.ID); p == nil || p.Status != "proposed" {
		t.Errorf("Proposals() entry = %+v, want proposed", p)
	}

	dup := model.PatternProposal{
		ID: got.ID, Type: "PERSON_NAME", Kind: model.KindAllowList,
		AllowList: []string{"Moriarty"}, Rationale: "another fictional character",
	}
	if _, err := r.AddProposal(dup); err == nil {
		t.Error("duplicate ID accepted, want error")
	}
	if n := len(r.Proposals()); n != 1 {
		t.Errorf("%d proposals after duplicate add, want 1", n)
	}
}

// An allow-list artifact that could never be approved is refused when it is
// authored, so it never sits in the queue as a suppression awaiting a click.
func TestAddProposalRejectsUnapprovableAllowList(t *testing.T) {
	r := newTestRunner(t, "")
	for _, terms := range [][]string{nil, {" padded"}, {"two\nlines"}, {strings.Repeat("x", maxAllowTermLen+1)}, nTerms(maxAllowTerms + 1)} {
		_, err := r.AddProposal(model.PatternProposal{
			ID: "bad-allow", Type: "PERSON_NAME", Kind: model.KindAllowList,
			AllowList: terms, Rationale: "should never land",
		})
		if err == nil {
			t.Errorf("allow list %q accepted, want error", terms)
		}
	}
	if p := proposalByID(r, "bad-allow"); p != nil {
		t.Errorf("unapprovable proposal present in Proposals(): %+v", p)
	}
}

func TestAddProposalAllowListApprovesIntoPack(t *testing.T) {
	r := newTestRunner(t, "")
	before := r.store.GetPatternPack()

	p, err := r.AddProposal(model.PatternProposal{
		ID: "allow-holmes", Type: "PERSON_NAME", Kind: model.KindAllowList,
		AllowList: []string{"Sherlock Holmes"}, Rationale: "fictional character in sample prompts",
	})
	if err != nil {
		t.Fatal(err)
	}
	pack, err := r.ApproveProposal(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != before.Version+1 {
		t.Errorf("pack version = %d, want %d", pack.Version, before.Version+1)
	}
	var rule *model.PatternRule
	for i := range pack.Rules {
		if pack.Rules[i].ID == "allow-holmes" {
			rule = &pack.Rules[i]
		}
	}
	if rule == nil || rule.Kind != model.KindAllowList || len(rule.AllowList) != 1 || rule.AllowList[0] != "Sherlock Holmes" {
		t.Fatalf("pack rule = %+v, want allow_list [Sherlock Holmes]", rule)
	}
	if got := r.store.GetPatternPack().Version; got != pack.Version {
		t.Errorf("store pack version = %d, want published %d", got, pack.Version)
	}
	if q := proposalByID(r, "allow-holmes"); q == nil || q.Status != "approved" {
		t.Errorf("proposal after approve = %+v, want approved", q)
	}
}
