package config

import (
	"strings"
	"testing"
)

func validEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GATEWAY_UPSTREAMS", `{"anthropic":{"base_url":"https://api.anthropic.com","credential_ref":"env:ANTHROPIC_UPSTREAM_KEY"}}`)
}

func TestLoadDefaults(t *testing.T) {
	validEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:8125" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.FailMode != "closed" {
		t.Errorf("FailMode = %q, want closed by default", cfg.FailMode)
	}
	if len(cfg.Detectors) != 1 || cfg.Detectors[0] != "regex" {
		t.Errorf("Detectors = %v", cfg.Detectors)
	}
	if cfg.MaxBodyBytes != 10<<20 {
		t.Errorf("MaxBodyBytes = %d", cfg.MaxBodyBytes)
	}
	if up, ok := cfg.Upstreams["anthropic"]; !ok || up.CredentialRef != "env:ANTHROPIC_UPSTREAM_KEY" {
		t.Errorf("Upstreams = %+v", cfg.Upstreams)
	}
	if cfg.AdvertiseURL != "http://127.0.0.1:8125" {
		t.Errorf("AdvertiseURL = %q, want derived from listen addr", cfg.AdvertiseURL)
	}
}

func TestLoadAdvertiseURLOverride(t *testing.T) {
	validEnv(t)
	t.Setenv("GATEWAY_ADVERTISE_URL", "http://gateway:8125")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AdvertiseURL != "http://gateway:8125" {
		t.Errorf("AdvertiseURL = %q", cfg.AdvertiseURL)
	}

	t.Setenv("GATEWAY_ADVERTISE_URL", "not-a-url")
	if _, err := Load(); err == nil {
		t.Error("want error for relative advertise URL")
	}
}

func TestLoadRejectsInlineSecretKey(t *testing.T) {
	t.Setenv("GATEWAY_UPSTREAMS", `{"anthropic":{"base_url":"https://api.anthropic.com","api_key":"sk-ant-inline"}}`)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "inline secret") {
		t.Fatalf("want inline-secret rejection, got %v", err)
	}
}

func TestLoadRejectsRawCredentialAsRef(t *testing.T) {
	t.Setenv("GATEWAY_UPSTREAMS", `{"anthropic":{"base_url":"https://api.anthropic.com","credential_ref":"sk-ant-raw-key"}}`)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "raw credential") {
		t.Fatalf("want raw-credential rejection, got %v", err)
	}
}

func TestLoadRejectsNonRefCredential(t *testing.T) {
	t.Setenv("GATEWAY_UPSTREAMS", `{"anthropic":{"base_url":"https://api.anthropic.com","credential_ref":"plaintext"}}`)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "not a secret reference") {
		t.Fatalf("want ref-scheme rejection, got %v", err)
	}
}

// A base_url is the other place a credential fits: the package doc promises an
// inline credential anywhere in the config kills the boot, and for a while that
// promise only inspected credential_ref.
func TestLoadRejectsCredentialInBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		wantErr string
	}{
		{"userinfo", "https://svc:sk-live-REALSECRET@api.anthropic.com", "userinfo credentials"},
		{"query param", "https://api.example.com/v1?api_key=sk-live-REALSECRET", "inline credential"},
		{"percent-encoded query param", "https://api.example.com/v1?api_key=%73k-live-REALSECRET", "inline credential"},
		{"clean", "https://api.example.com/v1?api-version=2026-03-10", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GATEWAY_UPSTREAMS", `{"anthropic":{"base_url":"`+tc.baseURL+`","credential_ref":"env:ANTHROPIC_UPSTREAM_KEY"}}`)
			_, err := Load()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want %q rejection, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestLoadRequiresUpstreams(t *testing.T) {
	t.Setenv("GATEWAY_UPSTREAMS", "")
	if _, err := Load(); err == nil {
		t.Fatal("want error when GATEWAY_UPSTREAMS unset")
	}
}

func TestLoadRejectsUnknownDetector(t *testing.T) {
	validEnv(t)
	t.Setenv("GATEWAY_DETECTORS", "regex,comprehend")
	if _, err := Load(); err == nil {
		t.Fatal("want error for unknown detector")
	}
}

func TestLoadRejectsBadAction(t *testing.T) {
	validEnv(t)
	t.Setenv("GATEWAY_REDACT_ACTION", "tokenize")
	if _, err := Load(); err == nil {
		t.Fatal("want error: tokenize is G3, not in this slice")
	}
}
