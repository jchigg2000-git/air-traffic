package harness

// Durable flywheel state under AIRTRAFFIC_DATA_DIR (default ./data/harness,
// gitignored): the ratchet series must survive restarts or it isn't a
// ratchet, and the promoted corpus is the accumulated regression set.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

type persister struct{ dir string }

// writeAtomic writes data to a sibling temp file and renames it into place,
// so an interrupted write cannot leave a half-parsed file behind — the same
// temp-file + rename the keystore and policy persisters use
// (internal/store/keystore_persist.go, internal/store/policy_persist.go).
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func newPersister(dir string) (*persister, error) {
	if err := os.MkdirAll(filepath.Join(dir, "corpus"), 0o755); err != nil {
		return nil, fmt.Errorf("harness data dir: %w", err)
	}
	return &persister{dir: dir}, nil
}

func (p *persister) loadRatchet() []model.RatchetPoint {
	raw, err := os.ReadFile(filepath.Join(p.dir, "ratchet.jsonl"))
	if err != nil {
		return nil
	}
	var points []model.RatchetPoint
	dec := json.NewDecoder(bytes.NewReader(raw))
	for dec.More() {
		var pt model.RatchetPoint
		if dec.Decode(&pt) != nil {
			break
		}
		points = append(points, pt)
	}
	return points
}

func (p *persister) appendRatchet(pt model.RatchetPoint) error {
	f, err := os.OpenFile(filepath.Join(p.dir, "ratchet.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(pt)
}

func (p *persister) loadCorpus() []model.CorpusEntry {
	files, _ := filepath.Glob(filepath.Join(p.dir, "corpus", "*.json"))
	var entries []model.CorpusEntry
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var e model.CorpusEntry
		if json.Unmarshal(raw, &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries
}

func (p *persister) saveCorpusEntry(e model.CorpusEntry) error {
	raw, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(p.dir, "corpus", e.ID+".json"), raw, 0o644)
}

type patternsFile struct {
	Pack      model.PatternPack       `json:"pack"`
	Proposals []model.PatternProposal `json:"proposals"`
}

// loadPatterns reads the persisted pack. A missing file is the first boot and
// not an error; an unreadable or malformed one is, because the empty pack
// returned in its place is indistinguishable from an owner having deleted
// every rule — and it is what every gateway would pull next.
func (p *persister) loadPatterns() (model.PatternPack, []model.PatternProposal, error) {
	empty := model.PatternPack{Rules: []model.PatternRule{}}
	raw, err := os.ReadFile(filepath.Join(p.dir, "patterns.json"))
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil, nil
	}
	if err != nil {
		return empty, nil, fmt.Errorf("reading patterns.json: %w", err)
	}
	var pf patternsFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		return empty, nil, fmt.Errorf("parsing patterns.json: %w", err)
	}
	return pf.Pack, pf.Proposals, nil
}

func (p *persister) savePatterns(pack model.PatternPack, proposals []model.PatternProposal) error {
	raw, err := json.MarshalIndent(patternsFile{Pack: pack, Proposals: proposals}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(p.dir, "patterns.json"), raw, 0o644)
}
