package model

// The gateway keystore: apps, the keys issued to them, and the versioned
// snapshot the gateway pulls.
//
// Until this existed the gateway's entire notion of "who is calling" was a
// comma-separated env var resolved once at boot, so a request carried no
// principal and every caller shared one redaction posture. The keystore's
// payoff is therefore NOT authentication — the env var already authenticated —
// it is attribution (which app sent this?) and scoping (this app's policy,
// not everyone's).
//
// Secrets are never stored. An issued key exists in plaintext exactly once, in
// the response to the issuing call; what persists is a SHA-256 digest.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

// App is the parent principal: one client application that talks to the
// gateway. Policy attaches HERE rather than to individual keys — an app may
// hold many keys (one per user id, agent instance, or CI job) and they all
// share the app's posture.
type App struct {
	ID   string `json:"id"` // slug, e.g. "hf-sandbox"
	Name string `json:"name"`
	// Baseline scopes the redaction action to this app. Empty means inherit
	// the globally applied policy, which is what every caller does today, so
	// an app that never sets it behaves exactly as before the keystore.
	Baseline  string    `json:"baseline,omitempty"`
	Disabled  bool      `json:"disabled,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// APIKey is one issued gateway credential.
//
// Subject is a free-form tag distinguishing keys within an app — a user id, an
// agent instance, a CI job. It is deliberately unstructured: an OIDC `sub`
// claim maps into this same field later without changing the record, so
// federation becomes a new issuance path rather than a new data model.
type APIKey struct {
	ID      string `json:"id"` // kid; safe to log, appears in every report
	AppID   string `json:"app_id"`
	Subject string `json:"subject,omitempty"`
	Label   string `json:"label,omitempty"`
	// Digest is the hex SHA-256 of the secret half. The secret itself is 192
	// bits from crypto/rand, not a password: there is no dictionary to attack,
	// so a slow KDF would buy nothing while adding latency to the proxy hot
	// path. No pepper either — distributing one to every gateway would
	// recreate the shared-secret problem it was meant to solve.
	Digest string `json:"digest"`
	// Routes limits which gateway dialects this key may call ("anthropic",
	// "openai"). Empty means every route.
	Routes    []string   `json:"routes,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// RevokedAt tombstones the key rather than deleting it: a revoked
	// credential that appears in an old report must still resolve to a name.
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// Active reports whether the key may authenticate a request at time now.
func (k APIKey) Active(now time.Time) bool {
	if k.RevokedAt != nil {
		return false
	}
	return k.ExpiresAt == nil || now.Before(*k.ExpiresAt)
}

// AllowsRoute reports whether the key may call the named gateway route.
func (k APIKey) AllowsRoute(route string) bool {
	if len(k.Routes) == 0 {
		return true
	}
	for _, r := range k.Routes {
		if r == route {
			return true
		}
	}
	return false
}

// KeySnapshot is the versioned keystore view the gateway pulls from
// GET /api/gateway/keys, on the same version-compare-and-swap contract as
// PatternPack. It carries digests and metadata, never a secret, so a leaked
// snapshot yields no usable credential.
type KeySnapshot struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Apps      []App     `json:"apps"`
	Keys      []APIKey  `json:"keys"`
}

// Key wire format: gwk_<kid>_<secret>.
//
// The underscore separator is load-bearing. Keys minted by scripts/dev-env.sh
// into GATEWAY_CLIENT_KEYS use the older `gwk-…` hyphen form, so the gateway
// can tell a keystore key from a legacy env key by shape alone and route the
// two verification paths without ambiguity.
const (
	KeyPrefix = "gwk"
	kidBytes  = 6  // 12 hex chars
	secBytes  = 24 // 192 bits, 32 base64url chars
)

// NewAPIKeySecret mints a key id and its plaintext secret. The caller is
// responsible for handing the plaintext to the issuer exactly once and never
// persisting or logging it.
func NewAPIKeySecret() (kid, plaintext string) {
	idb := make([]byte, kidBytes)
	secb := make([]byte, secBytes)
	if _, err := rand.Read(idb); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	if _, err := rand.Read(secb); err != nil {
		panic(err)
	}
	kid = hex.EncodeToString(idb)
	secret := base64.RawURLEncoding.EncodeToString(secb)
	return kid, KeyPrefix + "_" + kid + "_" + secret
}

// ParseAPIKey splits a presented credential into its key id and secret half.
// ok is false for anything that is not a keystore key, including the legacy
// hyphenated env keys — those fall through to the env-set check.
// SplitN, not Split: the secret is base64url, whose alphabet includes '_', so
// splitting on every separator would shatter roughly half of all valid keys.
// The key id is hex and the prefix is fixed, so only the first two separators
// are structural.
func ParseAPIKey(presented string) (kid, secret string, ok bool) {
	parts := strings.SplitN(presented, "_", 3)
	if len(parts) != 3 || parts[0] != KeyPrefix || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// DigestSecret hashes the secret half of a key for storage and comparison.
func DigestSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
