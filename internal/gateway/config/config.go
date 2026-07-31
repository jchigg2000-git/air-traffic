// Package config loads and validates the inference gateway's environment
// configuration. Secrets are accepted by reference only (env:NAME; vault:/kms:
// recognized for later slices) — an inline credential anywhere in the config
// kills the boot before the gateway binds a port.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"air-traffic/internal/redact"
)

// Upstream is one proxied vendor route: where to forward and which credential
// reference to resolve at call time.
type Upstream struct {
	BaseURL       string `json:"base_url"`
	CredentialRef string `json:"credential_ref"`
}

// Config is the gateway's full runtime configuration.
type Config struct {
	ListenAddr string
	// AdvertiseURL is the base URL peers should use to reach this gateway
	// (heartbeats carry it; the harness sends traffic to it). Defaults to
	// http://<ListenAddr>, which is wrong inside a container listening on
	// 0.0.0.0 — set GATEWAY_ADVERTISE_URL there.
	AdvertiseURL  string
	Upstreams     map[string]Upstream
	ClientKeysRef string
	// ControlPlaneKeyRef resolves the shared key this gateway presents on the
	// control plane's spine routes. Optional: an unset secret leaves the
	// gateway unauthenticated, which the control plane accepts only from
	// loopback callers.
	ControlPlaneKeyRef string
	Detectors          []string
	PresidioURL        string
	DetectorTimeout    time.Duration
	RedactAction       string // mask | block | per_policy
	FailMode           string // open | closed
	ControlPlaneURL    string
	ObsPushInterval    time.Duration
	PolicyPullInterval time.Duration
	MaxBodyBytes       int64
}

var (
	refScheme = regexp.MustCompile(`^(env|vault|kms):\S+$`)
	// Shapes of raw credentials that must never appear as a config value.
	rawSecret = regexp.MustCompile(`^(sk-|sk_live_|ghp_|gho_|github_pat_|AKIA|ASIA|xoxb-|ya29\.)`)
)

// Load reads and validates the gateway configuration from the environment.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:         env("GATEWAY_LISTEN_ADDR", "127.0.0.1:8125"),
		ClientKeysRef:      env("GATEWAY_CLIENT_KEYS_REF", "env:GATEWAY_CLIENT_KEYS"),
		ControlPlaneKeyRef: env("GATEWAY_CONTROL_PLANE_KEY_REF", "env:GATEWAY_CONTROL_PLANE_KEY"),
		PresidioURL:        env("GATEWAY_PRESIDIO_URL", "http://127.0.0.1:8126"),
		RedactAction:       env("GATEWAY_REDACT_ACTION", "mask"),
		FailMode:           env("GATEWAY_FAIL_MODE", "closed"),
		ControlPlaneURL:    env("GATEWAY_CONTROL_PLANE_URL", "http://127.0.0.1:8122"),
	}

	cfg.AdvertiseURL = env("GATEWAY_ADVERTISE_URL", "http://"+cfg.ListenAddr)
	if u, err := url.Parse(cfg.AdvertiseURL); err != nil || u.Scheme == "" || u.Host == "" {
		return Config{}, fmt.Errorf("GATEWAY_ADVERTISE_URL %q is not an absolute URL", cfg.AdvertiseURL)
	}

	upstreams, err := parseUpstreams(os.Getenv("GATEWAY_UPSTREAMS"))
	if err != nil {
		return Config{}, err
	}
	cfg.Upstreams = upstreams

	if !refScheme.MatchString(cfg.ClientKeysRef) {
		return Config{}, fmt.Errorf("GATEWAY_CLIENT_KEYS_REF %q is not a secret reference (want env:NAME)", cfg.ClientKeysRef)
	}
	if !refScheme.MatchString(cfg.ControlPlaneKeyRef) {
		return Config{}, fmt.Errorf("GATEWAY_CONTROL_PLANE_KEY_REF %q is not a secret reference (want env:NAME)", cfg.ControlPlaneKeyRef)
	}

	for _, d := range strings.Split(env("GATEWAY_DETECTORS", "regex"), ",") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if d != "regex" && d != "presidio" {
			return Config{}, fmt.Errorf("GATEWAY_DETECTORS: unknown detector %q (want regex, presidio)", d)
		}
		cfg.Detectors = append(cfg.Detectors, d)
	}
	if len(cfg.Detectors) == 0 {
		return Config{}, fmt.Errorf("GATEWAY_DETECTORS resolved to an empty chain")
	}

	switch cfg.RedactAction {
	case "mask", "block", "per_policy":
	default:
		return Config{}, fmt.Errorf("GATEWAY_REDACT_ACTION %q (want mask, block, per_policy)", cfg.RedactAction)
	}
	switch cfg.FailMode {
	case "open", "closed":
	default:
		return Config{}, fmt.Errorf("GATEWAY_FAIL_MODE %q (want open, closed)", cfg.FailMode)
	}

	cfg.DetectorTimeout, err = envMillis("GATEWAY_DETECTOR_TIMEOUT_MS", 800)
	if err != nil {
		return Config{}, err
	}
	cfg.ObsPushInterval, err = envDuration("GATEWAY_OBS_PUSH_INTERVAL", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg.PolicyPullInterval, err = envDuration("GATEWAY_POLICY_PULL_INTERVAL", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg.MaxBodyBytes, err = envInt64("GATEWAY_MAX_BODY_BYTES", 10<<20)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseUpstreams(raw string) (map[string]Upstream, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("GATEWAY_UPSTREAMS is required: JSON route → {base_url, credential_ref}")
	}
	// First pass untyped, so inline-secret keys are caught even when the typed
	// decode would reject them as unknown fields with a less pointed message.
	var loose map[string]map[string]any
	if err := json.Unmarshal([]byte(raw), &loose); err != nil {
		return nil, fmt.Errorf("GATEWAY_UPSTREAMS is not valid JSON: %w", err)
	}
	for route, obj := range loose {
		if redact.HasPlaintextSecretKey(obj) {
			return nil, fmt.Errorf("GATEWAY_UPSTREAMS[%s] carries an inline secret; store the credential elsewhere and pass a reference (credential_ref: env:NAME)", route)
		}
	}

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var upstreams map[string]Upstream
	if err := dec.Decode(&upstreams); err != nil {
		return nil, fmt.Errorf("GATEWAY_UPSTREAMS: %w", err)
	}
	if len(upstreams) == 0 {
		return nil, fmt.Errorf("GATEWAY_UPSTREAMS is empty")
	}
	for route, up := range upstreams {
		u, err := url.Parse(up.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("GATEWAY_UPSTREAMS[%s].base_url %q is not an absolute URL", route, up.BaseURL)
		}
		if rawSecret.MatchString(up.CredentialRef) {
			return nil, fmt.Errorf("GATEWAY_UPSTREAMS[%s].credential_ref looks like a raw credential; refusing to start (want env:NAME)", route)
		}
		if !refScheme.MatchString(up.CredentialRef) {
			return nil, fmt.Errorf("GATEWAY_UPSTREAMS[%s].credential_ref %q is not a secret reference (want env:NAME)", route, up.CredentialRef)
		}
	}
	return upstreams, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envMillis(key string, fallback int) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return time.Duration(fallback) * time.Millisecond, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s %q is not a positive integer of milliseconds", key, v)
	}
	return time.Duration(n) * time.Millisecond, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s %q is not a positive Go duration (e.g. 5s)", key, v)
	}
	return d, nil
}

func envInt64(key string, fallback int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s %q is not a positive integer", key, v)
	}
	return n, nil
}
