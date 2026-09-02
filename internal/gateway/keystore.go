package gateway

// Edge-side keystore: the snapshot pulled from the control plane, and the
// verification the proxy runs against it on every request.
//
// Verification is local by design. The alternative — asking the control plane
// per request — would add a network hop to a path that currently advertises
// 12-23 ms of added latency, and would break the property this package's doc
// comment stakes out: the gateway holds no cross-request state and keeps
// serving when the control plane is down.

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

// principal is who the authenticated caller is. Legacy GATEWAY_CLIENT_KEYS
// callers get envPrincipal, so every request has one and downstream code never
// needs a nil check.
type principal struct {
	AppID    string
	KeyID    string
	Subject  string
	Baseline string // "" = inherit the globally applied policy
}

// envPrincipal attributes the pre-keystore path. GATEWAY_CLIENT_KEYS is still
// required at boot and still authenticates: the keystore is additive, and the
// demo stack, dev-env.sh and the e2e script all depend on that path working
// unchanged.
var envPrincipal = principal{AppID: "env", KeyID: "env"}

type principalCtxKey struct{}

func withPrincipal(ctx context.Context, p principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// principalFrom returns the authenticated caller. The zero principal is
// returned for requests that never passed requireClientKey (health probes).
func principalFrom(ctx context.Context) principal {
	p, _ := ctx.Value(principalCtxKey{}).(principal)
	return p
}

// keySnapshot is the pulled keystore in lookup form.
type keySnapshot struct {
	version int
	apps    map[string]model.App
	keys    map[string]model.APIKey
}

func newKeySnapshot(snap model.KeySnapshot) *keySnapshot {
	ks := &keySnapshot{
		version: snap.Version,
		apps:    make(map[string]model.App, len(snap.Apps)),
		keys:    make(map[string]model.APIKey, len(snap.Keys)),
	}
	for _, a := range snap.Apps {
		ks.apps[a.ID] = a
	}
	for _, k := range snap.Keys {
		ks.keys[k.ID] = k
	}
	return ks
}

// authenticate resolves a presented credential to a principal for the named
// route. It tries the keystore first (keys are recognizable by shape) and
// falls back to the boot-time env key set.
func (s *Server) authenticate(presented, route string) (principal, bool) {
	if presented == "" {
		return principal{}, false
	}
	if kid, secret, ok := model.ParseAPIKey(presented); ok {
		return s.authenticateKeystore(kid, secret, route)
	}
	// The env key set is a handful of entries, so a linear constant-time scan
	// costs nothing and keeps this path timing-safe like the keystore digest
	// and spine-key checks.
	for k := range s.clientKeys {
		if subtle.ConstantTimeCompare([]byte(k), []byte(presented)) == 1 {
			return envPrincipal, true
		}
	}
	return principal{}, false
}

func (s *Server) authenticateKeystore(kid, secret, route string) (principal, bool) {
	snap := s.keys.Load()
	if snap == nil {
		return principal{}, false // no snapshot pulled yet; nothing to check against
	}
	k, ok := snap.keys[kid]
	if !ok {
		return principal{}, false // the key id is not secret, so a fast miss is fine
	}
	if subtle.ConstantTimeCompare([]byte(model.DigestSecret(secret)), []byte(k.Digest)) != 1 {
		return principal{}, false
	}
	if !k.Active(time.Now().UTC()) || !k.AllowsRoute(route) {
		return principal{}, false
	}
	app, ok := snap.apps[k.AppID]
	if !ok || app.Disabled {
		return principal{}, false
	}
	return principal{AppID: app.ID, KeyID: k.ID, Subject: k.Subject, Baseline: app.Baseline}, true
}

// allAppsEnforce reports whether every app carrying its own baseline resolves
// to an enforcing action. It backs the heartbeat's coverage claim: an app
// scoped to monitor-only means this gateway is not gating all of its traffic,
// however strict the global default is.
func (s *Server) allAppsEnforce() bool {
	snap := s.keys.Load()
	if snap == nil {
		return true
	}
	for _, app := range snap.apps {
		if app.Disabled || app.Baseline == "" {
			continue
		}
		if a, _, _ := s.resolveAction(principal{AppID: app.ID, Baseline: app.Baseline}); !enforces(a) {
			return false
		}
	}
	return true
}
