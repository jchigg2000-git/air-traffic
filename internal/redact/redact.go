// Package redact strips secrets from recorded headers, query params, and bodies.
package redact

import "strings"

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

// Headers returns a copy of h with sensitive values masked.
func Headers(h map[string][]string) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		if isSensitive(k) {
			out[k] = []string{placeholder}
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

// Query returns a copy of q with sensitive values masked.
func Query(q map[string][]string) map[string][]string {
	out := make(map[string][]string, len(q))
	for k, v := range q {
		if isSensitive(k) {
			out[k] = []string{placeholder}
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

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
