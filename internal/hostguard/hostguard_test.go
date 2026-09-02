package hostguard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrap(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := Wrap(ok, []string{" Control-Plane ", ""})

	cases := []struct {
		name    string
		method  string
		path    string
		host    string
		headers map[string]string
		want    int
	}{
		// httptest.NewRequest defaults Host to example.com, which is exactly
		// why the guard is wired in main.go around Routes() and not inside it.
		{"unknown host", "GET", "/api/health", "example.com", nil, http.StatusMisdirectedRequest},
		{"loopback v4", "GET", "/api/health", "127.0.0.1:8122", nil, http.StatusOK},
		{"loopback v6", "GET", "/api/health", "[::1]:8122", nil, http.StatusOK},
		{"localhost", "POST", "/api/policies", "localhost:8122", nil, http.StatusOK},
		{"localhost subdomain", "GET", "/", "app.localhost", nil, http.StatusOK},
		{"allowed name, case-insensitive", "POST", "/api/gateway/leaks", "control-plane:8122", nil, http.StatusOK},
		{"empty host", "GET", "/", "", nil, http.StatusMisdirectedRequest},
		{"cross-site write via Sec-Fetch-Site", "POST", "/api/policies", "127.0.0.1:8122",
			map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"cross-site read is fine", "GET", "/api/policies", "127.0.0.1:8122",
			map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusOK},
		{"cross-site _harness read mutates, refused", "GET", "/synthetic/anthropic/_harness/reset", "127.0.0.1:8122",
			map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"foreign Origin write", "POST", "/api/credentials", "127.0.0.1:8122",
			map[string]string{"Origin": "http://attacker.example"}, http.StatusForbidden},
		{"null Origin write fails closed", "POST", "/api/credentials", "127.0.0.1:8122",
			map[string]string{"Origin": "null"}, http.StatusForbidden},
		{"same-origin write", "POST", "/api/credentials", "127.0.0.1:8122",
			map[string]string{"Origin": "http://127.0.0.1:8122"}, http.StatusOK},
		{"same-site fetch", "POST", "/api/credentials", "localhost:8122",
			map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://localhost:8122"}, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			req.Host = c.host
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("%s %s Host=%q %v → %d, want %d", c.method, c.path, c.host, c.headers, rec.Code, c.want)
			}
		})
	}
}
