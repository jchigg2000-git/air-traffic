package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"air-traffic/internal/model"
)

var keystoreAdminRoutes = []struct {
	method, path, body string
}{
	{"GET", "/api/apps", ""},
	{"POST", "/api/apps", `{"id":"evil"}`},
	{"PATCH", "/api/apps/demo", `{"disabled":true}`},
	{"GET", "/api/apps/demo/keys", ""},
	{"POST", "/api/apps/demo/keys", `{"subject":"attacker"}`},
	{"DELETE", "/api/keys/abc123", ""},
}

// Key minting is the most dangerous surface here, so a remote caller is
// refused outright — including on the read routes, since the key list names
// every app and subject the deployment serves.
func TestKeystoreAdminRejectsRemoteCallers(t *testing.T) {
	_, _, h := newTestServer(t)
	for _, rt := range keystoreAdminRoutes {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		req.RemoteAddr = "203.0.113.7:44321"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s from remote = %d, want 401", rt.method, rt.path, rec.Code)
		}
	}
}

// Setting the spine key must NOT open the keystore: the gateway holds that
// key, and a gateway able to mint its own credentials makes the keystore
// pointless.
func TestSpineKeyDoesNotUnlockKeystoreAdmin(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.SetSpineKey("spine-test-key")
	h := srv.Routes()

	req := httptest.NewRequest("POST", "/api/apps", strings.NewReader(`{"id":"evil"}`))
	req.RemoteAddr = "203.0.113.7:44321"
	req.Header.Set("X-Air-Traffic-Key", "spine-test-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("spine key opened the keystore: %d", rec.Code)
	}
}

func TestIssueKeyReturnsSecretExactlyOnce(t *testing.T) {
	_, _, h := newTestServer(t)

	rec := do(h, "POST", "/api/apps", `{"id":"hf-sandbox","baseline":"fintech"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create app = %d: %s", rec.Code, rec.Body)
	}

	rec = do(h, "POST", "/api/apps/hf-sandbox/keys", `{"subject":"user-42","routes":["openai"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("issue = %d: %s", rec.Code, rec.Body)
	}
	var issued struct {
		Key    model.APIKey `json:"key"`
		Secret string       `json:"secret"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if issued.Secret == "" {
		t.Fatal("issuance returned no secret")
	}
	if issued.Key.Digest != "" {
		t.Error("issuance response leaked the digest")
	}
	kid, _, ok := model.ParseAPIKey(issued.Secret)
	if !ok || kid != issued.Key.ID {
		t.Errorf("secret %q does not carry key id %q", issued.Secret, issued.Key.ID)
	}

	// The list route must never hand the secret back.
	rec = do(h, "GET", "/api/apps/hf-sandbox/keys", "")
	if strings.Contains(rec.Body.String(), issued.Secret) {
		t.Error("the key list returned the plaintext secret")
	}

	// Revocation reaches the gateway through the snapshot, so the version has
	// to move.
	before := snapshotVersion(t, h)
	rec = do(h, "DELETE", "/api/keys/"+issued.Key.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", rec.Code, rec.Body)
	}
	if after := snapshotVersion(t, h); after <= before {
		t.Errorf("snapshot version %d did not advance past %d on revoke", after, before)
	}
}

func TestCreateAppRejectsUnknownBaseline(t *testing.T) {
	_, _, h := newTestServer(t)
	rec := do(h, "POST", "/api/apps", `{"id":"app","baseline":"not-a-baseline"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown baseline = %d, want 400", rec.Code)
	}
}

func TestIssueKeyRejectsUnknownRoute(t *testing.T) {
	_, _, h := newTestServer(t)
	do(h, "POST", "/api/apps", `{"id":"app"}`)
	rec := do(h, "POST", "/api/apps/app/keys", `{"routes":["gemini"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown route = %d, want 400", rec.Code)
	}
}

// do issues a loopback-originated admin request.
func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func snapshotVersion(t *testing.T, h http.Handler) int {
	t.Helper()
	rec := do(h, "GET", "/api/gateway/keys", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot = %d: %s", rec.Code, rec.Body)
	}
	var payload struct {
		Snapshot model.KeySnapshot `json:"snapshot"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return payload.Snapshot.Version
}
