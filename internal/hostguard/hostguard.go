// Package hostguard is the browser-facing guard both binaries wrap their
// handler tree in: a Host allow-list (defeats DNS rebinding) and a
// cross-site refusal for state-changing requests (defeats CSRF driven
// through the operator's own browser). Non-browser clients — curl, the
// gateway, the harness — send none of the headers it reads and pass untouched.
package hostguard

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Wrap guards next. allowed lists extra hostnames accepted in Host (port
// ignored, case-insensitive); loopback IPs and localhost always pass.
func Wrap(next http.Handler, allowed []string) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, h := range allowed {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			set[h] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, set) {
			refuse(w, http.StatusMisdirectedRequest, "unrecognised Host header (loopback and localhost always pass; add others to AIRTRAFFIC_ALLOWED_HOSTS / GATEWAY_ALLOWED_HOSTS)")
			return
		}
		if crossSite(r) && (!isRead(r.Method) || strings.Contains(r.URL.Path, "/_harness/")) {
			refuse(w, http.StatusForbidden, "cross-site browser request refused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func refuse(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}` + "\n"))
}

func hostname(hostport string) string {
	h := hostport
	if hp, _, err := net.SplitHostPort(hostport); err == nil {
		h = hp
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

func hostAllowed(host string, set map[string]struct{}) bool {
	h := hostname(host)
	if h == "" {
		return false
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
		return true
	}
	_, ok := set[h]
	return ok
}

// crossSite is true when the browser says so (Sec-Fetch-Site) or when an
// Origin is present that is not this server. An unparseable or "null"
// Origin is treated as cross-site — fail closed.
func crossSite(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return true
	}
	return !strings.EqualFold(u.Host, r.Host)
}

func isRead(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}
