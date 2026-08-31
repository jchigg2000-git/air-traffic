package gateway

// Pinning tests for the credential path. These are deliberately narrow: they
// cover the ways a key must FAIL to authenticate, the compatibility of the
// legacy env path, and the per-app action resolution — the three things whose
// regression would be silent and expensive.

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/gateway/config"
	"github.com/jchigg2000-git/air-traffic/internal/model"
)

func newKeystoreGateway(t *testing.T, upstreamURL, redactAction string) *Server {
	t.Helper()
	t.Setenv("GATEWAY_UPSTREAMS", fmt.Sprintf(`{"anthropic":{"base_url":%q,"credential_ref":"env:TEST_UPSTREAM_CRED"}}`, upstreamURL))
	t.Setenv("TEST_UPSTREAM_CRED", testUpstreamCred)
	t.Setenv("GATEWAY_CLIENT_KEYS_REF", "env:TEST_CLIENT_KEYS")
	t.Setenv("TEST_CLIENT_KEYS", testClientKey)
	t.Setenv("GATEWAY_REDACT_ACTION", redactAction)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	gw, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return gw
}

// mintKey builds a stored record plus the plaintext a client would present.
func mintKey(appID, subject string, routes []string) (model.APIKey, string) {
	kid, plaintext := model.NewAPIKeySecret()
	_, secret, _ := model.ParseAPIKey(plaintext)
	return model.APIKey{
		ID: kid, AppID: appID, Subject: subject,
		Digest: model.DigestSecret(secret), Routes: routes,
		CreatedAt: time.Now().UTC(),
	}, plaintext
}

func TestAuthenticateKeystoreRejections(t *testing.T) {
	gw := newKeystoreGateway(t, "http://127.0.0.1:1", "mask")

	past := time.Now().UTC().Add(-time.Hour)
	good, goodPlain := mintKey("hf-sandbox", "user-42", nil)
	expired, expiredPlain := mintKey("hf-sandbox", "", nil)
	expired.ExpiresAt = &past
	revoked, revokedPlain := mintKey("hf-sandbox", "", nil)
	revoked.RevokedAt = &past
	scoped, scopedPlain := mintKey("hf-sandbox", "", []string{"openai"})
	offApp, offAppPlain := mintKey("retired", "", nil)

	gw.keys.Store(newKeySnapshot(model.KeySnapshot{
		Version: 1,
		Apps: []model.App{
			{ID: "hf-sandbox", Name: "hf-sandbox"},
			{ID: "retired", Name: "retired", Disabled: true},
		},
		Keys: []model.APIKey{good, expired, revoked, scoped, offApp},
	}))

	// A valid key that is not in the snapshot's shape at all: same kid, wrong
	// secret. This is the case a plain map lookup would have gotten wrong.
	_, wrongSecretPlain := mintKey("hf-sandbox", "", nil)
	wrongSecret := model.KeyPrefix + "_" + good.ID + "_" + strings.SplitN(wrongSecretPlain, "_", 3)[2]

	cases := []struct {
		name      string
		presented string
		route     string
		wantOK    bool
		wantApp   string
	}{
		{"valid key", goodPlain, "anthropic", true, "hf-sandbox"},
		{"legacy env key", testClientKey, "anthropic", true, "env"},
		{"empty", "", "anthropic", false, ""},
		{"unknown kid", model.KeyPrefix + "_deadbeefcafe_nope", "anthropic", false, ""},
		{"right kid wrong secret", wrongSecret, "anthropic", false, ""},
		{"expired", expiredPlain, "anthropic", false, ""},
		{"revoked", revokedPlain, "anthropic", false, ""},
		{"out of route scope", scopedPlain, "anthropic", false, ""},
		{"in route scope", scopedPlain, "openai", true, "hf-sandbox"},
		{"disabled app", offAppPlain, "anthropic", false, ""},
		{"not a key at all", "hunter2", "anthropic", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := gw.authenticate(tc.presented, tc.route)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && p.AppID != tc.wantApp {
				t.Errorf("app_id = %q, want %q", p.AppID, tc.wantApp)
			}
		})
	}

	if p, _ := gw.authenticate(goodPlain, "anthropic"); p.Subject != "user-42" || p.KeyID != good.ID {
		t.Errorf("attribution = %+v, want subject user-42 and key_id %s", p, good.ID)
	}
}

// A gwk_-shaped key presented before any snapshot has been pulled must fail:
// there is nothing to check it against, and treating "unknown" as "fine" would
// turn a control-plane outage into an open door.
func TestKeystoreKeyRejectedBeforeFirstPull(t *testing.T) {
	gw := newKeystoreGateway(t, "http://127.0.0.1:1", "mask")
	_, plaintext := mintKey("hf-sandbox", "", nil)
	if _, ok := gw.authenticate(plaintext, "anthropic"); ok {
		t.Error("authenticated a keystore key with no snapshot loaded")
	}
	if _, ok := gw.authenticate(testClientKey, "anthropic"); !ok {
		t.Error("env key must still authenticate with no snapshot loaded")
	}
}

func TestActionForScopesBaselinePerApp(t *testing.T) {
	gw := newKeystoreGateway(t, "http://127.0.0.1:1", "per_policy")
	baselines := map[string]model.Baseline{
		"general_saas": {ID: "general_saas", PIIRedaction: "off"},
		"fintech":      {ID: "fintech", PIIRedaction: "on"},
		"healthcare":   {ID: "healthcare", PIIRedaction: "on+phi", ZDR: "enforced"},
	}
	gw.baselines.Store(&baselines)
	gw.policyAction.Store(actionDetect)
	gw.policyBaseline.Store("general_saas")

	// An app with no baseline of its own resolves exactly as before the
	// keystore existed. This is the compatibility bar.
	if got, base := gw.actionFor(principal{AppID: "env"}); got != actionDetect || base != "general_saas" {
		t.Errorf("unscoped app = (%q, %q), want (detect, general_saas)", got, base)
	}
	if got, base := gw.actionFor(principal{AppID: "strict", Baseline: "fintech"}); got != actionMask || base != "fintech" {
		t.Errorf("fintech app = (%q, %q), want (mask, fintech)", got, base)
	}
	// The pre-coverage gate applies per app too: healthcare without an
	// attested ZDR blocks, even while the global posture is monitor-only.
	if got, _ := gw.actionFor(principal{AppID: "clinic", Baseline: "healthcare"}); got != actionBlock {
		t.Errorf("healthcare app = %q, want block", got)
	}
	// A baseline the gateway has not pulled falls back to the global action
	// rather than inventing a posture.
	if got, _ := gw.actionFor(principal{AppID: "ghost", Baseline: "nonexistent"}); got != actionDetect {
		t.Errorf("unknown baseline = %q, want the global detect", got)
	}
}

// A pinned GATEWAY_REDACT_ACTION still wins over every app baseline: config is
// the strictest source and per-app scoping must not be able to loosen it.
func TestPinnedConfigActionOverridesAppBaseline(t *testing.T) {
	gw := newKeystoreGateway(t, "http://127.0.0.1:1", "block")
	baselines := map[string]model.Baseline{"general_saas": {ID: "general_saas", PIIRedaction: "off"}}
	gw.baselines.Store(&baselines)
	if got, _ := gw.actionFor(principal{AppID: "loose", Baseline: "general_saas"}); got != actionBlock {
		t.Errorf("action = %q, want the pinned block", got)
	}
}

// The heartbeat must not claim enforcement while any app is scoped to
// monitor-only, however strict the global default is.
func TestHeartbeatCoverageAccountsForScopedApps(t *testing.T) {
	gw := newKeystoreGateway(t, "http://127.0.0.1:1", "per_policy")
	baselines := map[string]model.Baseline{
		"general_saas": {ID: "general_saas", PIIRedaction: "off"},
		"fintech":      {ID: "fintech", PIIRedaction: "on"},
	}
	gw.baselines.Store(&baselines)
	gw.policyAction.Store(actionMask)

	gw.keys.Store(newKeySnapshot(model.KeySnapshot{
		Version: 1,
		Apps:    []model.App{{ID: "strict", Baseline: "fintech"}},
	}))
	if !gw.allAppsEnforce() {
		t.Error("every app enforcing, want coverage claimed")
	}

	gw.keys.Store(newKeySnapshot(model.KeySnapshot{
		Version: 2,
		Apps:    []model.App{{ID: "strict", Baseline: "fintech"}, {ID: "watched", Baseline: "general_saas"}},
	}))
	if gw.allAppsEnforce() {
		t.Error("an app scoped to detect must stop the gateway claiming enforcement")
	}
}

// End to end through the real handler chain: the 401 arrives in the caller's
// own dialect, and a good key reaches the upstream with attribution attached.
func TestKeystoreKeyEndToEnd(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","content":[]}`)
	}))
	defer upstream.Close()

	gw := newKeystoreGateway(t, upstream.URL, "mask")
	key, plaintext := mintKey("hf-sandbox", "user-42", nil)
	gw.keys.Store(newKeySnapshot(model.KeySnapshot{
		Version: 1,
		Apps:    []model.App{{ID: "hf-sandbox", Name: "hf-sandbox"}},
		Keys:    []model.APIKey{key},
	}))
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	body := `{"model":"claude-test","messages":[{"role":"user","content":"hello"}]}`
	post := func(cred string) *http.Response {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+cred)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		return resp
	}

	resp := post("gwk_000000000000_bogus")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad key status = %d, want 401", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), `"type":"error"`) {
		t.Errorf("401 is not in the Anthropic error envelope: %s", raw)
	}

	resp2 := post(plaintext)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("good key status = %d, want 200", resp2.StatusCode)
	}

	reports := gw.audits.drain()
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	got := reports[0]
	if got.AppID != "hf-sandbox" || got.KeyID != key.ID || got.Subject != "user-42" {
		t.Errorf("attribution = app %q key %q subject %q", got.AppID, got.KeyID, got.Subject)
	}
}

// The plaintext key must never reach a log line or an audit record. This is
// the keystore's half of the guard TestNoRedactedValueInLogsOrAudit enforces
// for detected values.
func TestNoKeyMaterialInLogsOrAudit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","content":[]}`)
	}))
	defer upstream.Close()

	var logs strings.Builder
	gw := newKeystoreGateway(t, upstream.URL, "mask")
	gw.log = slog.New(slog.NewTextHandler(&logs, nil))

	key, plaintext := mintKey("hf-sandbox", "user-42", nil)
	gw.keys.Store(newKeySnapshot(model.KeySnapshot{
		Version: 1,
		Apps:    []model.App{{ID: "hf-sandbox"}},
		Keys:    []model.APIKey{key},
	}))
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages",
		strings.NewReader(`{"model":"claude-test","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()

	_, secret, _ := model.ParseAPIKey(plaintext)
	for _, forbidden := range []string{plaintext, secret, key.Digest} {
		if strings.Contains(logs.String(), forbidden) {
			t.Errorf("log line carries key material: %s", logs.String())
		}
	}
	// The subject is a human identifier and is deliberately kept out of logs
	// even though it rides the report.
	if strings.Contains(logs.String(), "user-42") {
		t.Errorf("log line carries the key subject: %s", logs.String())
	}
	for _, rep := range gw.audits.drain() {
		if strings.Contains(fmt.Sprint(rep), secret) {
			t.Error("audit record carries the key secret")
		}
	}
}
