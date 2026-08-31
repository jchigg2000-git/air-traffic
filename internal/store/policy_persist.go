package store

// Durable applied policy under AIRTRAFFIC_DATA_DIR, the same shape as the
// keystore beside it (keystore_persist.go): one small JSON file, rewritten
// whole, temp-file + rename so an interrupted write cannot leave a
// half-parsed policy behind.
//
// Why this exists (DECISIONS.md 2026-08-15, "Policy persists"): SetPolicy used
// to assign a pointer and nothing else. A control-plane restart therefore
// discarded the applied baseline silently while the gateway kept enforcing the
// action it had already pulled — the two halves disagreeing, with nothing
// comparing them. The deployed stack runs GATEWAY_REDACT_ACTION=per_policy, so
// that was the live configuration, not a latent one.
//
// Deliberately NOT persisted, per the same decision: observations, gateway
// reports, drift and audit. Those are a time series, and a durable one is what
// the rejected third-party-dependency fork would have bought.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

const policyFileName = "policy.json"

type policyPersister interface {
	save(model.Policy) error
}

type policyFile struct{ path string }

func (f policyFile) save(p model.Policy) error {
	buf, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding policy: %w", err)
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return fmt.Errorf("writing policy: %w", err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		return fmt.Errorf("committing policy: %w", err)
	}
	return nil
}

func (f policyFile) load() (*model.Policy, error) {
	raw, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return nil, nil // first boot; no policy applied yet
	}
	if err != nil {
		return nil, fmt.Errorf("reading policy: %w", err)
	}
	var p model.Policy
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", f.path, err)
	}
	return &p, nil
}

// EnablePolicyPersistence loads any previously applied policy from dir and
// installs the write-through file for subsequent applies.
//
// The failure posture differs from the keystore's on purpose. A corrupt
// keystore is fatal, because booting with an empty one silently invalidates
// every issued key and presents to clients as an unexplained 401 storm. A
// corrupt policy is NOT fatal: the caller gets the error to log, boot
// continues with no policy applied, and the operator re-applies from the Rigor
// Console — which is strictly better than a control plane that will not start
// and therefore cannot be used to fix itself.
func (s *Store) EnablePolicyPersistence(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("policy dir: %w", err)
	}
	f := policyFile{path: filepath.Join(dir, policyFileName)}
	p, loadErr := f.load()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.policyFile = f
	if loadErr != nil {
		return loadErr
	}
	if p != nil {
		s.policy = p
	}
	return nil
}

// PolicyPersistError reports the last write-through failure, if any. Surfaced
// on the status route: a policy that has silently stopped persisting looks
// fine right up until the restart that loses it.
func (s *Store) PolicyPersistError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policyPersistErr
}
