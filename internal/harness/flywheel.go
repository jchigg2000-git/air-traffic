package harness

// Flywheel v0 — honest and small: misses promote to a durable synthetic
// corpus (no surrogate needed; the generator IS synthetic), and are matched
// against a CURATED candidate library to become human-approvable pattern
// proposals. No learned regexes, no NER fine-tuning — the local no-GPU
// ceiling is recognizer configuration, and the UI copy says so.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"time"

	"air-traffic/internal/model"
)

// candidate is one curated pattern the flywheel may propose when misses of
// its shape appear. Compiled once at init; a candidate that fails to compile
// is a programmer error.
type candidate struct {
	ID         string
	Type       string
	Regex      string
	Confidence float64
	Rationale  string
	re         *regexp.Regexp
}

var candidateLibrary = compileCandidates([]candidate{
	{
		ID: "ssn-bare-context", Type: "SSN",
		Regex:      `(?i)\b(?:SSN|social security(?: number)?)(?:\s+on\s+file)?[:\s]+\d{9}\b`,
		Confidence: 0.7,
		Rationale:  "bare 9-digit SSNs after an SSN context word — the deliberate v0 miss",
	},
	{
		ID: "mrn-bare-context", Type: "MRN",
		Regex:      `(?i)\b(?:MRN|medical record(?: number)?)[:\s]+\d{7,9}\b`,
		Confidence: 0.7,
		Rationale:  "bare medical record numbers after an MRN context word",
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

// flywheel promotes missed results into the corpus and derives proposals.
// Returns the number of newly promoted corpus entries. Caller does NOT hold
// r.mu.
func (r *Runner) flywheel(run *model.HarnessRun, results []model.HarnessResult) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := map[string]bool{}
	for _, e := range r.corpus {
		seen[hashText(e.Text)] = true
	}

	promoted := 0
	candidateHits := map[string]int{}
	unmatchedTypes := map[string]int{}

	for _, res := range results {
		if len(res.MissedTypes) == 0 || res.Err != "" {
			continue
		}
		h := hashText(res.Content)
		if !seen[h] {
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

		// Would a curated candidate have caught this miss? Match the
		// candidate against the content and require overlap with a missed
		// truth span of its type.
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
				unmatchedTypes[missType]++
			}
		}
	}

	r.upsertProposals(run.ID, candidateHits, unmatchedTypes)
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

// upsertProposals refreshes proposal rows: curated candidates with hits
// become approvable proposals; miss types with no candidate surface as
// status "manual" — the honest limit of v0. Caller holds r.mu.
func (r *Runner) upsertProposals(runID string, candidateHits map[string]int, unmatchedTypes map[string]int) {
	existing := map[string]*model.PatternProposal{}
	for i := range r.proposals {
		existing[r.proposals[i].ID] = &r.proposals[i]
	}

	for _, cand := range candidateLibrary {
		hits := candidateHits[cand.ID]
		if hits == 0 {
			continue
		}
		if p, ok := existing[cand.ID]; ok {
			if p.Status == "proposed" {
				p.SampleMisses += hits
				p.SourceRun = runID
			}
			continue // approved/rejected proposals are settled
		}
		r.proposals = append(r.proposals, model.PatternProposal{
			ID: cand.ID, Type: cand.Type, Regex: cand.Regex, Confidence: cand.Confidence,
			Rationale: cand.Rationale, SampleMisses: hits, Status: "proposed",
			SourceRun: runID, CreatedAt: time.Now().UTC(),
		})
	}

	types := make([]string, 0, len(unmatchedTypes))
	for t := range unmatchedTypes {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		id := "manual-" + t
		if p, ok := existing[id]; ok {
			p.SampleMisses += unmatchedTypes[t]
			continue
		}
		r.proposals = append(r.proposals, model.PatternProposal{
			ID: id, Type: t, Status: "manual",
			Rationale:    "needs a new recognizer — no curated candidate covers these misses (v0 limit; author one manually)",
			SampleMisses: unmatchedTypes[t], SourceRun: runID, CreatedAt: time.Now().UTC(),
		})
	}

	if err := r.persist.savePatterns(r.store.GetPatternPack(), r.proposals); err != nil {
		r.log.Warn("proposals persist failed", "error", err)
	}
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
	prop.Status = "approved"

	pack := r.store.GetPatternPack()
	pack.Rules = append(pack.Rules, model.PatternRule{
		ID: prop.ID, Type: prop.Type, Regex: prop.Regex, Confidence: prop.Confidence,
		Rationale: prop.Rationale, AddedAt: time.Now().UTC(),
	})
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
		After:          map[string]any{"pattern_id": prop.ID, "pack_version": pack.Version},
		RequestID:      model.NewUUID(),
	})
	r.log.Info("pattern approved", "id", prop.ID, "pack_version", pack.Version)
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
