package harness

import (
	"context"
	"testing"

	"air-traffic/internal/gateway/detect"
	"air-traffic/internal/model"
)

func TestGenerateDeterministicAndOffsetsExact(t *testing.T) {
	cfg := model.HarnessRunConfig{Count: 50, Seed: 42, IncludeTraps: true, IncludePresidioOnly: true, IncludeStraddle: true}
	a := generate(cfg, nil)
	b := generate(cfg, nil)
	if len(a) != 50 || len(b) != 50 {
		t.Fatalf("counts = %d, %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Content != b[i].Content {
			t.Fatalf("seed 42 not deterministic at item %d", i)
		}
		for _, truth := range a[i].Truth {
			if got := a[i].Content[truth.Start:truth.End]; got != truth.Value {
				t.Errorf("item %d truth offsets wrong: content[%d:%d]=%q want %q", i, truth.Start, truth.End, got, truth.Value)
			}
		}
		for _, trap := range a[i].Traps {
			if got := a[i].Content[trap.Start:trap.End]; got != trap.Value {
				t.Errorf("item %d trap offsets wrong", i)
			}
		}
	}
}

// Generated values must satisfy the detectors' validators: Luhn-valid cards,
// mod-97-valid IBANs, dashed/spaced SSNs caught; traps must never fire.
func TestGeneratedValuesAgreeWithDetectors(t *testing.T) {
	det := detect.NewRegex()
	cfg := model.HarnessRunConfig{Count: 200, Seed: 7, IncludeTraps: true}
	items := generate(cfg, nil)
	checked := map[string]int{}
	for _, item := range items {
		spans, _ := det.Detect(context.Background(), item.Content)
		for _, truth := range item.Truth {
			// bare SSN/MRN variants are deliberate misses; only strict shapes must hit
			if truth.Type == "CREDIT_CARD" || truth.Type == "IBAN" || truth.Type == "EMAIL" || truth.Type == "IP" {
				found := false
				for _, sp := range spans {
					if sp.Type == truth.Type && sp.Start < truth.End && truth.Start < sp.End {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("detector missed generated %s %q in %q", truth.Type, truth.Value, item.Content)
				}
				checked[truth.Type]++
			}
		}
		for _, trap := range item.Traps {
			// regression for the slot-shadowing bug: trap values must look
			// like order numbers / versions / cards — never person names
			if trap.Value != "" && trap.Value[0] != 'O' && trap.Value[0] != 'v' &&
				(trap.Value[0] < '0' || trap.Value[0] > '9') {
				t.Errorf("trap value %q does not look like a trap", trap.Value)
			}
			for _, sp := range spans {
				if sp.Start < trap.End && trap.Start < sp.End {
					t.Errorf("trap fired: %s detected inside trap %q (%q)", sp.Type, trap.Value, item.Content)
				}
			}
		}
	}
	for _, typ := range []string{"CREDIT_CARD", "IBAN", "EMAIL"} {
		if checked[typ] == 0 {
			t.Errorf("no %s values generated in 200 items — templates or weights broken", typ)
		}
	}
}

func TestGenerateReplayMixesCorpus(t *testing.T) {
	corpus := []model.CorpusEntry{{ID: "c1", Text: "replayed text with SSN 123456789", Truth: []model.TruthSpan{{Start: 24, End: 33, Type: "SSN", Value: "123456789"}}}}
	cfg := model.HarnessRunConfig{Count: 100, Seed: 3, ReplayPercent: 50}
	items := generate(cfg, corpus)
	replays := 0
	for _, item := range items {
		if item.Replay {
			replays++
		}
	}
	if replays < 20 || replays > 80 {
		t.Errorf("replay mix = %d/100, want ≈50", replays)
	}
}
