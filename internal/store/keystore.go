package store

// Keystore state: apps, their issued keys, and the monotonic version the
// gateway compares against to decide whether to reload.
//
// Unlike everything else in this package, the keystore is NOT allowed to be
// purely in-memory: observations and reports can be lost to a container
// recreate, credentials cannot. EnableKeystorePersistence wires a
// write-through file (keystore_persist.go); without it the store is
// memory-only, which is what tests use.

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"air-traffic/internal/model"
)

// Bounds on owner-authored strings. These land in the gateway's report ring
// via attribution, so they get the same clamping discipline as anything else
// that crosses that boundary (see clampReport in internal/server).
const (
	maxAppNameLen = 120
	maxSubjectLen = 200
	maxLabelLen   = 120
	maxKeyRoutes  = 8
)

// appIDPattern keeps app ids URL-safe and log-safe: they appear in route
// paths, in every gateway report, and in structured log lines.
var appIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// keyPersister writes the keystore through to durable storage after every
// mutation. nil means memory-only.
type keyPersister interface {
	save(model.KeySnapshot) error
}

// ---- apps ----

// CreateApp registers a new client application. Baseline is validated by the
// caller (the store cannot import internal/policy without a cycle).
func (s *Store) CreateApp(a model.App) (model.App, error) {
	if !appIDPattern.MatchString(a.ID) {
		return model.App{}, fmt.Errorf("%w: app id must match %s", ErrInvalid, appIDPattern)
	}
	if a.Name == "" {
		a.Name = a.ID
	}
	if len(a.Name) > maxAppNameLen {
		return model.App{}, fmt.Errorf("%w: name exceeds %d bytes", ErrInvalid, maxAppNameLen)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.apps[a.ID]; exists {
		return model.App{}, fmt.Errorf("%w: app %q already exists", ErrInvalid, a.ID)
	}
	a.CreatedAt = time.Now().UTC()
	s.apps[a.ID] = a
	s.bumpKeysLocked()
	return a, nil
}

// UpdateApp applies the non-nil fields of a patch. Baseline is set to the
// empty string by passing a pointer to "" — that is how an app is returned to
// inheriting the global policy.
func (s *Store) UpdateApp(id string, baseline *string, disabled *bool, name *string) (model.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.apps[id]
	if !ok {
		return model.App{}, fmt.Errorf("%w: app %q", ErrNotFound, id)
	}
	if baseline != nil {
		a.Baseline = *baseline
	}
	if disabled != nil {
		a.Disabled = *disabled
	}
	if name != nil {
		if *name == "" || len(*name) > maxAppNameLen {
			return model.App{}, fmt.Errorf("%w: name must be 1..%d bytes", ErrInvalid, maxAppNameLen)
		}
		a.Name = *name
	}
	s.apps[id] = a
	s.bumpKeysLocked()
	return a, nil
}

func (s *Store) GetApp(id string) (model.App, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.apps[id]
	return a, ok
}

func (s *Store) ListApps() []model.App {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedAppsLocked(s.apps)
}

// ---- keys ----

// IssueKey mints a credential for an app and returns the record plus the
// plaintext key. The plaintext is returned exactly once, here: only its digest
// is stored, so nothing can recover it afterwards.
func (s *Store) IssueKey(appID, subject, label string, routes []string, expiresAt *time.Time) (model.APIKey, string, error) {
	if len(subject) > maxSubjectLen {
		return model.APIKey{}, "", fmt.Errorf("%w: subject exceeds %d bytes", ErrInvalid, maxSubjectLen)
	}
	if len(label) > maxLabelLen {
		return model.APIKey{}, "", fmt.Errorf("%w: label exceeds %d bytes", ErrInvalid, maxLabelLen)
	}
	if len(routes) > maxKeyRoutes {
		return model.APIKey{}, "", fmt.Errorf("%w: at most %d routes", ErrInvalid, maxKeyRoutes)
	}
	if expiresAt != nil && !expiresAt.After(time.Now().UTC()) {
		return model.APIKey{}, "", fmt.Errorf("%w: expires_at is already in the past", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.apps[appID]; !ok {
		return model.APIKey{}, "", fmt.Errorf("%w: app %q", ErrNotFound, appID)
	}
	kid, plaintext := model.NewAPIKeySecret()
	_, secret, ok := model.ParseAPIKey(plaintext)
	if !ok { // unreachable: we just minted it in that shape
		return model.APIKey{}, "", fmt.Errorf("minted key failed its own parse")
	}
	k := model.APIKey{
		ID:        kid,
		AppID:     appID,
		Subject:   subject,
		Label:     label,
		Digest:    model.DigestSecret(secret),
		Routes:    routes,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}
	s.keys[kid] = k
	s.bumpKeysLocked()
	return k, plaintext, nil
}

// RevokeKey tombstones a key. The record is kept, not deleted: a revoked
// credential still has to resolve to a name when it turns up in an old report.
func (s *Store) RevokeKey(kid string) (model.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[kid]
	if !ok {
		return model.APIKey{}, fmt.Errorf("%w: key %q", ErrNotFound, kid)
	}
	if k.RevokedAt != nil {
		return k, nil // idempotent
	}
	now := time.Now().UTC()
	k.RevokedAt = &now
	s.keys[kid] = k
	s.bumpKeysLocked()
	return k, nil
}

// ListKeys returns key metadata for one app (or every app when appID is
// empty), newest first. Digests are zeroed: they are needed by the gateway,
// never by a human reading the admin API.
func (s *Store) ListKeys(appID string) []model.APIKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []model.APIKey{}
	for _, k := range s.keys {
		if appID != "" && k.AppID != appID {
			continue
		}
		k.Digest = ""
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// KeySnapshot is what the gateway pulls: apps plus keys WITH digests, since
// verification happens at the edge. Digests are not secrets — the keys they
// hash are 192 bits of crypto/rand — so this is safe to ship over the spine.
func (s *Store) KeySnapshot() model.KeySnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keySnapshotLocked()
}

func (s *Store) keySnapshotLocked() model.KeySnapshot {
	keys := make([]model.APIKey, 0, len(s.keys))
	for _, k := range s.keys {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })
	return model.KeySnapshot{
		Version:   s.keysVersion,
		UpdatedAt: s.keysUpdatedAt,
		Apps:      sortedAppsLocked(s.apps),
		Keys:      keys,
	}
}

// bumpKeysLocked advances the snapshot version and writes through to disk.
// Callers already hold the write lock.
//
// The file write happens under that lock. Key mutations are rare (admin
// actions only) and the file holds tens of records, so the simplicity is
// worth more here than the contention would cost.
func (s *Store) bumpKeysLocked() {
	s.keysVersion++
	s.keysUpdatedAt = time.Now().UTC()
	if s.keyFile == nil {
		return
	}
	if err := s.keyFile.save(s.keySnapshotLocked()); err != nil {
		// Surfaced rather than swallowed: a keystore that silently stops
		// persisting looks fine until the next restart drops every key.
		s.keysPersistErr = err
		return
	}
	s.keysPersistErr = nil
}

// KeystorePersistError reports the last write-through failure, for /api/health
// and the gateway status surface. nil means the keystore is durable.
func (s *Store) KeystorePersistError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keysPersistErr
}

func sortedAppsLocked(m map[string]model.App) []model.App {
	out := make([]model.App, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
