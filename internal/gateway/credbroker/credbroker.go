// Package credbroker resolves secret references (env:NAME today; vault:/kms:
// recognized but deferred) at call time. Resolved values are never stored on
// config structs and never logged.
package credbroker

import (
	"fmt"
	"os"
	"strings"
)

// Resolver turns a secret reference into its value.
type Resolver interface {
	Resolve(ref string) (string, error)
}

// New returns the MVP resolver (env: scheme only).
func New() Resolver { return envResolver{} }

type envResolver struct{}

func (envResolver) Resolve(ref string) (string, error) {
	scheme, name, ok := strings.Cut(ref, ":")
	if !ok || name == "" {
		return "", fmt.Errorf("secret ref %q is not scheme:name", ref)
	}
	switch scheme {
	case "env":
		v := os.Getenv(name)
		if v == "" {
			return "", fmt.Errorf("secret ref %q resolves to an empty value", ref)
		}
		return v, nil
	case "vault", "kms":
		return "", fmt.Errorf("secret scheme %q is recognized but deferred to a later slice (docs/plans/TODO-gateway-deferred.md)", scheme)
	default:
		return "", fmt.Errorf("unknown secret scheme %q", scheme)
	}
}
