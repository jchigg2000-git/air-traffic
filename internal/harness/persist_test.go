package harness

// The pattern pack is the one piece of flywheel state gateways enforce. A load
// that fails silently hands them an empty v0 pack at the next pull, which is
// indistinguishable from an owner deleting every rule.

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
)

func TestLoadPatternsSurfacesUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	p, err := newPersister(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Missing file is the first boot, not a failure.
	pack, _, err := p.loadPatterns()
	if err != nil {
		t.Fatalf("missing patterns.json should load clean: %v", err)
	}
	if len(pack.Rules) != 0 {
		t.Errorf("first-boot pack = %+v", pack)
	}

	if err := os.WriteFile(filepath.Join(dir, "patterns.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.loadPatterns(); err == nil {
		t.Fatal("malformed patterns.json loaded without error")
	}
}

func TestNewRunnerDoesNotPublishAnUnloadablePack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "patterns.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := store.New()
	live := model.PatternPack{Version: 3, Rules: []model.PatternRule{{ID: "r1", Type: "SSN", Regex: `\d{9}`}}}
	st.SetPatternPack(live)

	r, err := NewRunner(st, slog.New(slog.NewTextHandler(io.Discard, nil)), dir, "gwk-test", "")
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if r.PatternPersistError() == nil {
		t.Error("pattern load failure not reported")
	}
	if got := st.GetPatternPack(); got.Version != 3 || len(got.Rules) != 1 {
		t.Errorf("unloadable pack was published as %+v", got)
	}
}
