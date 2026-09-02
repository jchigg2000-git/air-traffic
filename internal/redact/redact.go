// Package redact strips secrets from recorded headers, query params, and bodies.
package redact

import (
	"regexp"
	"strings"
)

const placeholder = "[REDACTED]"

var sensitiveKeys = []string{
	"authorization", "cookie", "set-cookie", "x-api-key", "api-key", "api_key",
	"token", "access_token", "refresh_token", "secret", "client_secret",
	"password", "passwd", "private_key", "x-goog-api-key", "x-amz-security-token",
}

func isSensitive(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// mask returns a deep copy of m with sensitive values replaced by placeholder.
// Headers and Query share this body; both are maps of string to []string and
// the same key list applies to each.
func mask(m map[string][]string) map[string][]string {
	out := make(map[string][]string, len(m))
	for k, v := range m {
		if isSensitive(k) {
			out[k] = []string{placeholder}
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

// Headers returns a copy of h with sensitive values masked.
func Headers(h map[string][]string) map[string][]string { return mask(h) }

// Query returns a copy of q with sensitive values masked.
func Query(q map[string][]string) map[string][]string { return mask(q) }

// HasPlaintextSecretKey reports whether a decoded JSON object carries a key that
// looks like an inline plaintext secret (so credential writes can reject it).
func HasPlaintextSecretKey(body map[string]any) bool {
	for k, v := range body {
		lk := strings.ToLower(k)
		// secret_ref is the allowed reference form; anything else secret-ish is rejected.
		if lk == "secret_ref" {
			continue
		}
		for _, s := range []string{"secret", "token", "password", "api_key", "apikey", "private_key"} {
			if strings.Contains(lk, s) {
				return true
			}
		}
		if nested, ok := v.(map[string]any); ok {
			if HasPlaintextSecretKey(nested) {
				return true
			}
		}
	}
	return false
}

var (
	// secretRef is the only shape a credential may take anywhere it is stored
	// or configured: a reference the resolver dereferences at call time.
	secretRef = regexp.MustCompile(`^(env|vault|kms):\S+$`)
	// rawSecret is the set of vendor key prefixes that must never appear as a
	// value — the shapes a paste-by-mistake takes.
	rawSecret = regexp.MustCompile(`^(sk-|sk_live_|ghp_|gho_|github_pat_|AKIA|ASIA|xoxb-|ya29\.)`)
)

// IsSecretRef reports whether v is a credential reference (env:NAME,
// vault:PATH, kms:KEY) rather than a value.
func IsSecretRef(v string) bool { return secretRef.MatchString(v) }

// LooksLikeRawSecret reports whether v starts like a known vendor credential
// (sk-…, ghp_…, AKIA…): a value pasted where a reference belongs.
func LooksLikeRawSecret(v string) bool { return rawSecret.MatchString(v) }
