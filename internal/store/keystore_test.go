package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

func TestIssueKeyStoresDigestNotSecret(t *testing.T) {
	s := New()
	if _, err := s.CreateApp(model.App{ID: "hf-sandbox"}); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	key, plaintext, err := s.IssueKey("hf-sandbox", "user-42", "laptop", nil, nil)
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}

	kid, secret, ok := model.ParseAPIKey(plaintext)
	if !ok || kid != key.ID {
		t.Fatalf("issued key %q does not parse back to %q", plaintext, key.ID)
	}
	if key.Digest != model.DigestSecret(secret) {
		t.Error("stored digest does not match the issued secret")
	}
	// Nothing anywhere may hand the secret back a second time.
	for _, k := range s.ListKeys("hf-sandbox") {
		if k.Digest != "" {
			t.Error("ListKeys leaked a digest")
		}
	}
	snap := s.KeySnapshot()
	for _, k := range snap.Keys {
		if k.Digest == "" {
			t.Error("the gateway snapshot needs digests to verify against")
		}
	}
}

func TestRevokeTombstonesAndBumpsVersion(t *testing.T) {
	s := New()
	_, _ = s.CreateApp(model.App{ID: "app"})
	key, _, _ := s.IssueKey("app", "", "", nil, nil)
	before := s.KeySnapshot().Version

	revoked, err := s.RevokeKey(key.ID)
	if err != nil {
		t.Fatalf("RevokeKey: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Error("revoked key has no RevokedAt")
	}
	if got := s.KeySnapshot().Version; got <= before {
		t.Errorf("version = %d, want > %d — the gateway reloads on a version bump", got, before)
	}
	// The record survives: an old report still has to resolve to a name.
	if len(s.ListKeys("app")) != 1 {
		t.Error("revocation deleted the record instead of tombstoning it")
	}
	if _, err := s.RevokeKey(key.ID); err != nil {
		t.Errorf("revoke should be idempotent, got %v", err)
	}
}

func TestKeystoreValidation(t *testing.T) {
	s := New()
	if _, err := s.CreateApp(model.App{ID: "Not A Slug"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad app id error = %v, want ErrInvalid", err)
	}
	if _, err := s.CreateApp(model.App{ID: "app"}); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if _, err := s.CreateApp(model.App{ID: "app"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("duplicate app error = %v, want ErrInvalid", err)
	}
	if _, _, err := s.IssueKey("nope", "", "", nil, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("issue against unknown app error = %v, want ErrNotFound", err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	if _, _, err := s.IssueKey("app", "", "", nil, &past); !errors.Is(err, ErrInvalid) {
		t.Errorf("past expiry error = %v, want ErrInvalid", err)
	}
	if _, err := s.RevokeKey("nosuchkey"); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoke unknown key error = %v, want ErrNotFound", err)
	}
}

// Credentials must survive a restart. Everything else in this store may be
// lost to one; an issued key may not.
func TestKeystorePersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()

	first := New()
	if err := first.EnableKeystorePersistence(dir); err != nil {
		t.Fatalf("EnableKeystorePersistence: %v", err)
	}
	if _, err := first.CreateApp(model.App{ID: "hf-sandbox", Baseline: "fintech"}); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	key, plaintext, err := first.IssueKey("hf-sandbox", "user-42", "", []string{"openai"}, nil)
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}
	wantVersion := first.KeySnapshot().Version

	if _, err := os.Stat(filepath.Join(dir, keystoreFileName)); err != nil {
		t.Fatalf("keystore file not written: %v", err)
	}

	second := New()
	if err := second.EnableKeystorePersistence(dir); err != nil {
		t.Fatalf("reload: %v", err)
	}
	snap := second.KeySnapshot()
	if snap.Version != wantVersion {
		t.Errorf("version = %d, want %d", snap.Version, wantVersion)
	}
	if len(snap.Apps) != 1 || snap.Apps[0].Baseline != "fintech" {
		t.Errorf("apps = %+v, want the fintech-scoped app", snap.Apps)
	}
	if len(snap.Keys) != 1 || snap.Keys[0].ID != key.ID {
		t.Fatalf("keys = %+v, want the issued key", snap.Keys)
	}
	_, secret, _ := model.ParseAPIKey(plaintext)
	if snap.Keys[0].Digest != model.DigestSecret(secret) {
		t.Error("reloaded digest does not verify the original key")
	}
	if len(snap.Keys[0].Routes) != 1 || snap.Keys[0].Routes[0] != "openai" {
		t.Errorf("route scope lost across the reload: %+v", snap.Keys[0].Routes)
	}
}

// A keystore file that cannot be parsed is a hard boot failure, not an empty
// keystore: silently invalidating every issued key reads to a client exactly
// like an unexplained 401 storm.
func TestCorruptKeystoreFailsBoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, keystoreFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := New().EnableKeystorePersistence(dir); err == nil {
		t.Error("want a hard error on a corrupt keystore")
	}
}

func TestUpdateAppClearsBaseline(t *testing.T) {
	s := New()
	_, _ = s.CreateApp(model.App{ID: "app", Baseline: "fintech"})
	empty := ""
	app, err := s.UpdateApp("app", &empty, nil, nil)
	if err != nil {
		t.Fatalf("UpdateApp: %v", err)
	}
	if app.Baseline != "" {
		t.Errorf("baseline = %q, want cleared back to the global policy", app.Baseline)
	}
}
