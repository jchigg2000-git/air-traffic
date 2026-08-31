package store

// Durable keystore under AIRTRAFFIC_DATA_DIR, following the same shape as the
// harness's flywheel state (internal/harness/persist.go): a small JSON file in
// the data volume, rewritten whole on change.
//
// Whole-file rewrite is correct at this size — a keystore holds tens of
// records and only changes when an operator issues or revokes — and the
// temp-file + rename makes it atomic, so an interrupted write cannot leave a
// half-parsed credential store behind.
//
// Only digests reach the disk. This file is not itself a credential.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

const keystoreFileName = "keys.json"

type keystoreFile struct{ path string }

func (f keystoreFile) save(snap model.KeySnapshot) error {
	buf, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding keystore: %w", err)
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return fmt.Errorf("writing keystore: %w", err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		return fmt.Errorf("committing keystore: %w", err)
	}
	return nil
}

func (f keystoreFile) load() (model.KeySnapshot, error) {
	raw, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return model.KeySnapshot{}, nil // first boot
	}
	if err != nil {
		return model.KeySnapshot{}, fmt.Errorf("reading keystore: %w", err)
	}
	var snap model.KeySnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return model.KeySnapshot{}, fmt.Errorf("parsing %s: %w", f.path, err)
	}
	return snap, nil
}

// EnableKeystorePersistence loads any existing keystore from dir and installs
// the write-through file for subsequent mutations. A corrupt or unreadable
// file is a hard error: booting with an empty keystore would silently
// invalidate every issued key, which reads to a client exactly like a 401
// storm with no cause.
func (s *Store) EnableKeystorePersistence(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("keystore dir: %w", err)
	}
	f := keystoreFile{path: filepath.Join(dir, keystoreFileName)}
	snap, err := f.load()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range snap.Apps {
		s.apps[a.ID] = a
	}
	for _, k := range snap.Keys {
		s.keys[k.ID] = k
	}
	s.keysVersion = snap.Version
	s.keysUpdatedAt = snap.UpdatedAt
	s.keyFile = f
	return nil
}
