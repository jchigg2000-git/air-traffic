package harness

// Flywheel — honest and small: misses promote to a durable synthetic corpus
// (no surrogate needed; the generator IS synthetic) and become
// human-approvable proposals only when a safe artifact exists. Three
// artifact kinds, in order of preference:
//
//	regex      — a CURATED candidate library entry matched the miss
//	threshold  — Presidio saw the span but under the score gate (probe-proven)
//	deny_list  — Presidio never saw the span and the type is free text, so an
//	             exact-term list is the only safe artifact
//
// Anything else stays "manual" — the honest limit. No learned regexes, no
// NER fine-tuning: the local no-GPU ceiling is recognizer configuration, and
// the UI copy says so.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"air-traffic/internal/model"
)

const (
	maxProbesPerRun = 25
	probeTimeout    = 2500 * time.Millisecond
	maxDenyTerms    = 50
	maxDenyTermLen  = 80
	minGate         = 0.05
	maxGate         = 0.95
)

// freeTextTypes are the NER-only types where no vetted regex can exist —
// deny lists are their fallback artifact.
var freeTextTypes = map[string]bool{"PERSON_NAME": true, "ADDRESS": true}

// candidate is one curated pattern the flywheel may propose when misses of
// its shape appear. Compiled once at init; a candidate that fails to compile
// is a programmer error.
type candidate struct {
	ID         string
	Type       string
	Regex      string
	Confidence float64
	Rationale  string
	Context    []string // Presidio context words that boost this pattern
	re         *regexp.Regexp
}

var candidateLibrary = compileCandidates([]candidate{
	{
		ID: "ssn-bare-context", Type: "SSN",
		Regex:      `(?i)\b(?:SSN|social security(?: number)?)(?:\s+on\s+file)?[:\s]+\d{9}\b`,
		Confidence: 0.7,
		Rationale:  "bare 9-digit SSNs after an SSN context word — the deliberate v0 miss",
		Context:    []string{"ssn", "social security"},
	},
	{
		ID: "mrn-bare-context", Type: "MRN",
		Regex:      `(?i)\b(?:MRN|medical record(?: number)?)[:\s]+\d{7,9}\b`,
		Confidence: 0.7,
		Rationale:  "bare medical record numbers after an MRN context word",
		Context:    []string{"mrn", "medical record"},
	},
	{
		ID: "phone-dotted", Type: "PHONE",
		Regex:      `\b\d{3}\.\d{3}\.\d{4}\b`,
		Confidence: 0.7,
		Rationale:  "dot-separated US phone numbers",
	},
})

func compileCandidates(cands []candidate) []candidate {
	for i := range cands {
		cands[i].re = regexp.MustCompile(cands[i].Regex)
	}
	return cands
}

// uncoveredMiss is one missed type in one request that no curated regex
// candidate covers — raw material for config-shaped candidates.
type uncoveredMiss struct {
	typ     string
	content string
	truth   []model.TruthSpan
}

// configEvidence aggregates, per type, which artifact would honestly cover
// the uncovered misses. Threshold evidence requires a probe (raw scores);
// deny evidence requires only that the value is free text.
type configEvidence struct {
	gate            float64 // the gate in force when the evidence was gathered
	thresholdMisses int
	minScore        float64
	denyMisses      int
	denyTerms       []string
	manualMisses    int
}

// flywheel promotes missed results into the corpus and derives proposals.
// Returns the number of newly promoted corpus entries. Caller does NOT hold
// r.mu; probing happens before the lock is taken.
func (r *Runner) flywheel(run *model.HarnessRun, results []model.HarnessResult) int {
	// Phase 1 (lock-free): match misses against the curated regex candidates;
	// whatever they don't cover is material for config candidates.
	candidateHits := map[string]int{}
	var uncovered []uncoveredMiss
	for _, res := range results {
		if len(res.MissedTypes) == 0 || res.Err != "" {
			continue
		}
		for _, missType := range res.MissedTypes {
			matched := false
			for _, cand := range candidateLibrary {
				if cand.Type != missType {
					continue
				}
				if candidateCovers(cand, res.Content, res.Truth, missType) {
					candidateHits[cand.ID]++
					matched = true
				}
			}
			if !matched {
				uncovered = append(uncovered, uncoveredMiss{typ: missType, content: res.Content, truth: res.Truth})
			}
		}
	}

	// Phase 2 (lock-free, network): probe Presidio for raw-score evidence.
	evidence := r.synthesizeConfig(run.DetectorChain, uncovered)

	// Phase 3 (locked): promote to the corpus, upsert proposals.
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := map[string]bool{}
	for _, e := range r.corpus {
		seen[hashText(e.Text)] = true
	}
	promoted := 0
	for _, res := range results {
		if len(res.MissedTypes) == 0 || res.Err != "" {
			continue
		}
		h := hashText(res.Content)
		if seen[h] {
			continue
		}
		entry := model.CorpusEntry{
			ID: h[:16], Text: res.Content, Truth: res.Truth,
			SourceRun: run.ID, MissTypes: res.MissedTypes, AddedAt: time.Now().UTC(),
		}
		if err := r.persist.saveCorpusEntry(entry); err != nil {
			r.log.Warn("corpus persist failed", "error", err)
		} else {
			r.corpus = append(r.corpus, entry)
			seen[h] = true
			promoted++
		}
	}

	r.upsertProposals(run.ID, candidateHits, evidence)
	return promoted
}

func candidateCovers(cand candidate, content string, truths []model.TruthSpan, missType string) bool {
	for _, loc := range cand.re.FindAllStringIndex(content, -1) {
		for _, truth := range truths {
			if truth.Type == missType && loc[0] < truth.End && truth.Start < loc[1] {
				return true
			}
		}
	}
	return false
}

// synthesizeConfig turns uncovered misses into per-type config evidence.
// Presidio-side artifacts only act when the run's chain consulted the engine
// — otherwise approving one would be a lie and everything stays manual.
func (r *Runner) synthesizeConfig(chain string, uncovered []uncoveredMiss) map[string]*configEvidence {
	if len(uncovered) == 0 {
		return nil
	}
	gates := r.currentGates()
	ev := map[string]*configEvidence{}
	get := func(t string) *configEvidence {
		e := ev[t]
		if e == nil {
			e = &configEvidence{gate: gateFor(gates, t), minScore: 1}
			ev[t] = e
		}
		return e
	}

	if !strings.Contains(chain, "presidio") {
		for _, u := range uncovered {
			get(u.typ).manualMisses += missSpanCount(u)
		}
		return ev
	}

	probe := r.probe
	rawCache := map[string][]probeSpan{}
	probed := 0
	for _, u := range uncovered {
		e := get(u.typ)
		raw, haveRaw := rawCache[u.content]
		if !haveRaw && probe != nil && probed < maxProbesPerRun {
			probed++
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			spans, err := probe.analyzeRaw(ctx, u.content)
			cancel()
			if err != nil {
				probe = nil // one failure disables probing for this run
				r.log.Warn("presidio probe unavailable; deny-list evidence only", "error", err)
			} else {
				rawCache[u.content] = spans
				raw, haveRaw = spans, true
			}
		}
		for _, t := range u.truth {
			if t.Type != u.typ {
				continue
			}
			score, sawIt := bestOverlap(raw, t, u.typ)
			switch {
			case haveRaw && sawIt && score < e.gate:
				e.thresholdMisses++
				if score < e.minScore {
					e.minScore = score
				}
			case haveRaw && sawIt:
				// The engine already clears the gate here; the miss is
				// elsewhere in the chain — no config artifact provably fixes it.
				e.manualMisses++
			case freeTextTypes[u.typ] && validateDenyTerm(t.Value) == nil:
				e.denyMisses++
				e.denyTerms = appendUnique(e.denyTerms, t.Value)
			default:
				e.manualMisses++
			}
		}
	}
	return ev
}

// currentGates reads per-type score gates from the active pack (approved
// threshold rules); absent types sit at the engine default.
func (r *Runner) currentGates() map[string]float64 {
	gates := map[string]float64{}
	for _, rule := range r.store.GetPatternPack().Rules {
		if rule.Kind == model.KindThreshold && rule.Threshold > 0 {
			if g, ok := gates[rule.Type]; !ok || rule.Threshold < g {
				gates[rule.Type] = rule.Threshold
			}
		}
	}
	return gates
}

func gateFor(gates map[string]float64, t string) float64 {
	if g, ok := gates[t]; ok {
		return g
	}
	return model.PresidioDefaultScoreGate
}

func missSpanCount(u uncoveredMiss) int {
	n := 0
	for _, t := range u.truth {
		if t.Type == u.typ {
			n++
		}
	}
	if n == 0 {
		n = 1
	}
	return n
}

func bestOverlap(raw []probeSpan, truth model.TruthSpan, typ string) (float64, bool) {
	best, found := 0.0, false
	for _, sp := range raw {
		if sp.Type == typ && sp.Start < truth.End && truth.Start < sp.End {
			found = true
			if sp.Score > best {
				best = sp.Score
			}
		}
	}
	return best, found
}

// upsertProposals refreshes proposal rows: curated candidates with hits and
// config evidence become approvable proposals; misses with no artifact stay
// "manual" — the honest limit. A manual row whose type gains a config
// proposal flips to "superseded". Caller holds r.mu.
//
// NB: lookups scan r.proposals fresh on every call — a find result must be
// used before the next append (appends may reallocate the slice).
func (r *Runner) upsertProposals(runID string, candidateHits map[string]int, evidence map[string]*configEvidence) {
	find := func(id string) *model.PatternProposal {
		for i := range r.proposals {
			if r.proposals[i].ID == id {
				return &r.proposals[i]
			}
		}
		return nil
	}

	for _, cand := range candidateLibrary {
		hits := candidateHits[cand.ID]
		if hits == 0 {
			continue
		}
		if p := find(cand.ID); p != nil {
			if p.Status == "proposed" {
				p.SampleMisses += hits
				p.SourceRun = runID
			}
			continue // approved/rejected proposals are settled
		}
		r.proposals = append(r.proposals, model.PatternProposal{
			ID: cand.ID, Type: cand.Type, Kind: model.KindRegex, Regex: cand.Regex,
			Context: cand.Context, Confidence: cand.Confidence,
			Rationale: cand.Rationale, SampleMisses: hits, Status: "proposed",
			SourceRun: runID, CreatedAt: time.Now().UTC(),
		})
	}

	types := make([]string, 0, len(evidence))
	for t := range evidence {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		e := evidence[t]
		artifact := false
		if e.thresholdMisses > 0 {
			artifact = r.upsertThreshold(t, e, runID) || artifact
		}
		if len(e.denyTerms) > 0 {
			artifact = r.upsertDeny(t, e, runID) || artifact
		}
		if e.manualMisses > 0 {
			r.upsertManual(t, e.manualMisses, runID, find)
		}
		if artifact {
			if p := find("manual-" + t); p != nil && p.Status == "manual" {
				p.Status = "superseded"
			}
		}
	}

	if err := r.persist.savePatterns(r.store.GetPatternPack(), r.proposals); err != nil {
		r.log.Warn("proposals persist failed", "error", err)
	}
}

// upsertThreshold proposes lowering the type's score gate to accept the
// probed evidence. A settled row at or below the needed gate means the ask
// is already covered (approved) or already declined (rejected).
func (r *Runner) upsertThreshold(t string, e *configEvidence, runID string) bool {
	needed := proposeGate(e.minScore)
	var open *model.PatternProposal
	for i := range r.proposals {
		p := &r.proposals[i]
		if p.Kind != model.KindThreshold || p.Type != t {
			continue
		}
		switch p.Status {
		case "proposed":
			if open == nil {
				open = p
			}
		case "approved", "rejected":
			if p.Threshold <= needed {
				return false
			}
		}
	}
	if open != nil {
		if needed < open.Threshold {
			open.Threshold = needed
		}
		open.SampleMisses += e.thresholdMisses
		open.SourceRun = runID
		open.Rationale = thresholdRationale(t, e, open.Threshold)
		return true
	}
	r.proposals = append(r.proposals, model.PatternProposal{
		ID: r.freeProposalID("threshold-" + t), Type: t, Kind: model.KindThreshold,
		Threshold: needed, Rationale: thresholdRationale(t, e, needed),
		SampleMisses: e.thresholdMisses, Status: "proposed",
		SourceRun: runID, CreatedAt: time.Now().UTC(),
	})
	return true
}

// upsertDeny merges newly-missed terms into an open deny-list proposal for
// the type, or opens one. Terms already present in ANY deny row for the type
// are skipped: approved terms are covered, rejected terms stay rejected.
func (r *Runner) upsertDeny(t string, e *configEvidence, runID string) bool {
	known := map[string]bool{}
	var open *model.PatternProposal
	for i := range r.proposals {
		p := &r.proposals[i]
		if p.Kind != model.KindDenyList || p.Type != t {
			continue
		}
		for _, term := range p.DenyList {
			known[term] = true
		}
		if p.Status == "proposed" && open == nil {
			open = p
		}
	}
	var fresh []string
	for _, term := range e.denyTerms {
		if !known[term] {
			fresh = append(fresh, term)
		}
	}
	if len(fresh) == 0 {
		if open != nil { // same terms missed again: accumulate on the open row
			open.SampleMisses += e.denyMisses
			open.SourceRun = runID
		}
		return open != nil
	}
	if open != nil {
		if room := maxDenyTerms - len(open.DenyList); len(fresh) > room {
			r.log.Warn("deny-list proposal at term cap", "type", t, "dropped", len(fresh)-room)
			fresh = fresh[:room]
		}
		open.DenyList = append(open.DenyList, fresh...)
		open.SampleMisses += e.denyMisses
		open.SourceRun = runID
		open.Rationale = denyRationale(len(open.DenyList))
		return true
	}
	if len(fresh) > maxDenyTerms {
		r.log.Warn("deny-list proposal at term cap", "type", t, "dropped", len(fresh)-maxDenyTerms)
		fresh = fresh[:maxDenyTerms]
	}
	r.proposals = append(r.proposals, model.PatternProposal{
		ID: r.freeProposalID("deny-" + t), Type: t, Kind: model.KindDenyList,
		DenyList: fresh, Confidence: 1, Rationale: denyRationale(len(fresh)),
		SampleMisses: e.denyMisses, Status: "proposed",
		SourceRun: runID, CreatedAt: time.Now().UTC(),
	})
	return true
}

func (r *Runner) upsertManual(t string, misses int, runID string, find func(string) *model.PatternProposal) {
	id := "manual-" + t
	if p := find(id); p != nil {
		if p.Status == "manual" {
			p.SampleMisses += misses
		}
		return
	}
	r.proposals = append(r.proposals, model.PatternProposal{
		ID: id, Type: t, Status: "manual",
		Rationale:    "needs a new recognizer — no curated candidate covers these misses and no Presidio-side config would provably catch them (author one manually)",
		SampleMisses: misses, SourceRun: runID, CreatedAt: time.Now().UTC(),
	})
}

func (r *Runner) freeProposalID(base string) string {
	taken := map[string]bool{}
	for i := range r.proposals {
		taken[r.proposals[i].ID] = true
	}
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		if id := fmt.Sprintf("%s-%d", base, n); !taken[id] {
			return id
		}
	}
}

// proposeGate rounds the lowest observed score down to the 0.05 grid, one
// step lower — a gate that accepts the evidence with a little slack.
func proposeGate(minScore float64) float64 {
	g := math.Floor(minScore*20)/20 - 0.05
	if g < minGate {
		g = minGate
	}
	return math.Round(g*100) / 100
}

func thresholdRationale(t string, e *configEvidence, gate float64) string {
	return fmt.Sprintf("Presidio saw %d missed %s span(s) at scores down to %.2f — under the %.2f gate; lowering the gate to %.2f accepts them",
		e.thresholdMisses, t, e.minScore, e.gate, gate)
}

func denyRationale(terms int) string {
	return fmt.Sprintf("Presidio's NER never saw these spans; an exact-term deny list is the honest artifact — catches recurrences and corpus replays (%d terms)", terms)
}

// validateDenyTerm keeps deny terms plain text: Presidio compiles them into
// a regex verbatim, so metacharacters could smuggle patterns past review.
func validateDenyTerm(term string) error {
	if t := strings.TrimSpace(term); t == "" || t != term {
		return fmt.Errorf("deny term %q is empty or has surrounding whitespace", term)
	}
	if len(term) > maxDenyTermLen {
		return fmt.Errorf("deny term longer than %d bytes", maxDenyTermLen)
	}
	if strings.ContainsAny(term, `\.+*?()|[]{}^$`) {
		return fmt.Errorf("deny term %q contains regex metacharacters", term)
	}
	return nil
}

// ruleFromProposal validates a proposal's artifact and shapes the pack rule.
// Approval is the trust boundary: whatever passes here hot-reloads into
// every gateway, so each kind gets a hard check.
func ruleFromProposal(p model.PatternProposal) (model.PatternRule, error) {
	rule := model.PatternRule{
		ID: p.ID, Type: p.Type, Confidence: p.Confidence, Rationale: p.Rationale,
	}
	switch p.Kind {
	case "", model.KindRegex:
		if _, err := regexp.Compile(p.Regex); err != nil {
			return rule, fmt.Errorf("proposal %q regex: %w", p.ID, err)
		}
		rule.Kind = model.KindRegex
		rule.Regex = p.Regex
		rule.Context = p.Context
	case model.KindDenyList:
		if len(p.DenyList) == 0 || len(p.DenyList) > maxDenyTerms {
			return rule, fmt.Errorf("proposal %q: deny list must hold 1–%d terms", p.ID, maxDenyTerms)
		}
		for _, term := range p.DenyList {
			if err := validateDenyTerm(term); err != nil {
				return rule, fmt.Errorf("proposal %q: %w", p.ID, err)
			}
		}
		rule.Kind = model.KindDenyList
		rule.DenyList = p.DenyList
	case model.KindThreshold:
		if p.Threshold < minGate || p.Threshold > maxGate {
			return rule, fmt.Errorf("proposal %q: threshold %.2f outside [%.2f, %.2f]", p.ID, p.Threshold, minGate, maxGate)
		}
		rule.Kind = model.KindThreshold
		rule.Threshold = p.Threshold
	default:
		return rule, fmt.Errorf("proposal %q has unknown kind %q", p.ID, p.Kind)
	}
	return rule, nil
}

// ApproveProposal moves a proposal into the active pattern pack: version
// bump, store publish (gateways hot-reload on next pull), durable persist,
// audit event. Human-in-the-loop by design.
func (r *Runner) ApproveProposal(id string) (model.PatternPack, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var prop *model.PatternProposal
	for i := range r.proposals {
		if r.proposals[i].ID == id {
			prop = &r.proposals[i]
		}
	}
	if prop == nil {
		return model.PatternPack{}, fmt.Errorf("unknown proposal %q", id)
	}
	if prop.Status != "proposed" {
		return model.PatternPack{}, fmt.Errorf("proposal %q is %s, not approvable", id, prop.Status)
	}
	rule, err := ruleFromProposal(*prop)
	if err != nil {
		return model.PatternPack{}, err
	}
	prop.Status = "approved"
	rule.AddedAt = time.Now().UTC()

	pack := r.store.GetPatternPack()
	pack.Rules = append(pack.Rules, rule)
	pack.Version++
	pack.UpdatedAt = time.Now().UTC()
	r.store.SetPatternPack(pack)
	if err := r.persist.savePatterns(pack, r.proposals); err != nil {
		r.log.Warn("pattern persist failed", "error", err)
	}
	r.store.AddAudit(model.AuditEvent{
		Actor: "air-traffic:admin", Action: "gateway.pattern.approve",
		Resource: prop.Type, Plane: model.PlaneDataPolicy, Vendor: "anthropic",
		ControlSurface: model.DispProxyEnforced,
		After:          map[string]any{"pattern_id": prop.ID, "kind": rule.Kind, "pack_version": pack.Version},
		RequestID:      model.NewUUID(),
	})
	r.log.Info("pattern approved", "id", prop.ID, "kind", rule.Kind, "pack_version", pack.Version)
	return pack, nil
}

func (r *Runner) RejectProposal(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.proposals {
		if r.proposals[i].ID == id {
			if r.proposals[i].Status != "proposed" {
				return fmt.Errorf("proposal %q is %s", id, r.proposals[i].Status)
			}
			r.proposals[i].Status = "rejected"
			if err := r.persist.savePatterns(r.store.GetPatternPack(), r.proposals); err != nil {
				r.log.Warn("proposals persist failed", "error", err)
			}
			return nil
		}
	}
	return fmt.Errorf("unknown proposal %q", id)
}

// corpusVersion fingerprints the replay corpus + generator so ratchet points
// are comparable only when measured against the same material.
func corpusVersion(corpus []model.CorpusEntry) string {
	h := sha256.New()
	h.Write([]byte("gen/v1|"))
	ids := make([]string, 0, len(corpus))
	for _, e := range corpus {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		h.Write([]byte(id))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
