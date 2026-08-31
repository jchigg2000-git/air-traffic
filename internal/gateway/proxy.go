package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"air-traffic/internal/gateway/config"
	"air-traffic/internal/gateway/redact"
	"air-traffic/internal/model"
)

// hop-by-hop headers (RFC 9110 §7.6.1) are never forwarded in either direction.
var hopByHop = map[string]bool{
	"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true,
	"Proxy-Authorization": true, "Te": true, "Trailer": true,
	"Transfer-Encoding": true, "Upgrade": true,
}

// client credential headers are stripped before the upstream credential is set.
var clientAuthHeaders = map[string]bool{"Authorization": true, "X-Api-Key": true}

// dialect is one vendor wire format the gateway proxies. Everything that
// differs between the supported dialects lives here; the request pipeline in
// proxyRequest is shared, so a detector or policy change lands on every route
// at once and the two can't drift.
type dialect struct {
	// route is the key looked up in GATEWAY_UPSTREAMS, and the label the audit
	// record carries.
	route string
	// path is appended to the upstream base URL. The two ecosystems put the
	// version segment on different sides of that seam and we follow each one's
	// convention rather than inventing a third: an Anthropic base_url is the
	// bare host (…anthropic.com + /v1/messages), while an OpenAI-compatible
	// base_url already ends in /v1 (…huggingface.co/v1 + /chat/completions).
	path        string
	walk        func(map[string]any) []textField
	jsonUsage   func([]byte) (tokensIn, tokensOut int64)
	newScanner  func() usageScanner
	writeErr    func(w http.ResponseWriter, status int, errType, msg string)
	defaultAuth string
}

func anthropicDialect() dialect {
	return dialect{
		route:       "anthropic",
		path:        "/v1/messages",
		walk:        walkAnthropicBody,
		jsonUsage:   anthropicJSONUsage,
		newScanner:  func() usageScanner { return newAnthropicUsageScanner() },
		writeErr:    writeVendorError,
		defaultAuth: config.AuthAPIKey,
	}
}

func openAIDialect() dialect {
	return dialect{
		route:       "openai",
		path:        "/chat/completions",
		walk:        walkOpenAIBody,
		jsonUsage:   openAIJSONUsage,
		newScanner:  func() usageScanner { return newOpenAIUsageScanner() },
		writeErr:    writeOpenAIError,
		defaultAuth: config.AuthBearer,
	}
}

// requireClientKey authenticates the caller's gateway key (Bearer or
// x-api-key) against the keystore snapshot, falling back to the boot-time env
// key set. The resolved principal rides the request context from here, which
// is what makes per-app attribution and per-app policy possible downstream.
//
// The rejection is written in the caller's own dialect, so a client SDK can
// parse the 401 it gets back. It is deliberately the same message for an
// unknown, expired, revoked, out-of-scope and disabled-app key: telling a
// caller which of those it hit is telling them something about keys they do
// not hold.
func (s *Server) requireClientKey(d dialect, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-api-key")
		if key == "" {
			key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		p, ok := s.authenticate(key, d.route)
		if !ok {
			d.writeErr(w, http.StatusUnauthorized, "authentication_error", "invalid gateway key")
			return
		}
		next(w, r.WithContext(withPrincipal(r.Context(), p)))
	}
}

// action values the pipeline can run under: mask/block come from config or
// policy, detect is the log-only monitor mode. All three alias the shared
// contract constants so the hot path and the control plane's pre-commit
// preview can never mean different things by the same word.
//
// pass is gateway-local and currently unreachable — resolveAction only ever
// yields mask/block/detect, and no baseline derives it (model.GatewayAction).
// It is kept as the audit default and the name for a future skip-detection
// route, not because a live path produces it.
const (
	actionMask   = model.ActionMask
	actionBlock  = model.ActionBlock
	actionDetect = model.ActionDetect
	actionPass   = "pass"
)

// actionFor resolves the effective redaction action for one caller, and names
// the baseline that decided it.
//
// Precedence, strictest source first:
//
//	GATEWAY_REDACT_ACTION pinned  → that value, for everyone (unchanged)
//	the caller's app names a baseline → derived from THAT baseline
//	otherwise                     → the globally applied policy (unchanged)
//
// The middle case is the whole point of the keystore: before it, one gateway
// served one posture, so a client that needed monitor-only and a client that
// needed masking could not share a deployment. An app that sets no baseline
// resolves exactly as it did before — that is the compatibility bar.
func (s *Server) actionFor(p principal) (action, baseline string) {
	action, baseline, missing := s.resolveAction(p)
	if missing {
		// The app names a baseline this gateway has not pulled. We fall
		// through to the global action rather than inventing one — guessing
		// would silently apply the wrong posture to a scoped app.
		s.log.Warn("app baseline not in the pulled set; using the global action",
			"app_id", p.AppID, "baseline", p.Baseline)
	}
	return action, baseline
}

// resolveAction is actionFor without the logging, so the heartbeat can survey
// every app's posture without emitting a warning per app per beat.
func (s *Server) resolveAction(p principal) (action, baseline string, missingBaseline bool) {
	if s.cfg.RedactAction != "per_policy" {
		return s.cfg.RedactAction, "", false
	}
	if p.Baseline != "" {
		if bs := s.baselines.Load(); bs != nil {
			if b, ok := (*bs)[p.Baseline]; ok {
				return deriveAction(b, s.zdrAttested.Load()), b.ID, false
			}
		}
		missingBaseline = true
	}
	global, _ := s.policyBaseline.Load().(string)
	if v := s.policyAction.Load(); v != nil {
		return v.(string), global, missingBaseline
	}
	return actionMask, global, missingBaseline
}

// enforces reports whether an action actually gates traffic. detect and pass
// are monitoring, not enforcement.
func enforces(action string) bool { return action == actionMask || action == actionBlock }

// handleMessages is the Anthropic Messages proxy route.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, anthropicDialect())
}

// handleChatCompletions is the OpenAI-compatible chat-completions proxy route.
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, openAIDialect())
}

// proxyRequest runs one request through the pipeline: read → detect →
// redact/block → swap credential → forward → return (byte-faithful when
// nothing was redacted).
func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, d dialect) {
	start := time.Now()
	up, ok := s.cfg.Upstreams[d.route]
	if !ok {
		d.writeErr(w, http.StatusBadGateway, "api_error", "no upstream configured for route "+d.route)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		d.writeErr(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "body too large or unreadable")
		return
	}

	p := principalFrom(r.Context())
	action, baseline := s.actionFor(p)

	audit := RequestAudit{
		RequestID: r.Header.Get("X-Gateway-Request-Id"),
		Route:     d.route,
		Action:    actionPass,
		AppID:     p.AppID,
		KeyID:     p.KeyID,
		Subject:   p.Subject,
		Baseline:  baseline,
		At:        time.Now().UTC(),
	}

	outBody := body
	if action == actionPass {
		// The pass path never decodes the body, so attribution comes from a
		// minimal peek rather than a full parse. A body that will not parse
		// still forwards — pass means pass.
		audit.Model = peekModel(body)
	} else {
		detectStart := time.Now()
		var doc map[string]any
		if err := json.Unmarshal(body, &doc); err != nil {
			d.writeErr(w, http.StatusBadRequest, "invalid_request_error", "body is not valid JSON")
			return
		}
		if m, ok := doc["model"].(string); ok {
			audit.Model = m
		}
		var redactions []Redaction
		var detErrs []string
		mutated := false
		for _, f := range d.walk(doc) {
			spans, errs := s.chain.Run(r.Context(), f.text)
			for _, e := range errs {
				detErrs = append(detErrs, e.Error())
			}
			if len(spans) == 0 {
				continue
			}
			for _, sp := range spans {
				redactions = append(redactions, Redaction{
					Path: f.path, Type: sp.Type, Start: sp.Start, End: sp.End,
					Detector: sp.Detector, Confidence: sp.Confidence,
				})
			}
			if action == actionMask {
				f.set(redact.Mask(f.text, spans))
				mutated = true
			}
		}
		audit.Redactions = redactions
		audit.DetectorErrors = detErrs
		audit.FailModeTripped = len(detErrs) > 0
		audit.AddedLatencyMS = time.Since(detectStart).Milliseconds()

		// A detector failure means we cannot promise the filter ran. Closed
		// fails the request (a gate that silently opens is worse than none);
		// open forwards with whatever engines succeeded.
		if len(detErrs) > 0 && s.cfg.FailMode == "closed" {
			audit.Action = actionBlock
			audit.LatencyMS = time.Since(start).Milliseconds()
			s.record(audit, 0, 0)
			d.writeErr(w, http.StatusServiceUnavailable, "api_error",
				"detector unavailable and GATEWAY_FAIL_MODE=closed")
			return
		}

		if len(redactions) > 0 {
			switch action {
			case actionBlock:
				audit.Action = actionBlock
				audit.LatencyMS = time.Since(start).Milliseconds()
				s.record(audit, 0, 0)
				d.writeErr(w, http.StatusBadRequest, "invalid_request_error",
					"request blocked by gateway policy: detected "+typeSummary(redactions))
				return
			case actionMask:
				audit.Action = actionMask
				if mutated {
					rewritten, err := json.Marshal(doc)
					if err != nil {
						d.writeErr(w, http.StatusInternalServerError, "api_error", "rewrite failed")
						return
					}
					outBody = rewritten
				}
			case actionDetect:
				audit.Action = actionDetect
			}
		}
	}

	cred, err := s.creds.Resolve(up.CredentialRef)
	if err != nil {
		s.log.Error("credential resolution failed", "route", d.route, "ref", up.CredentialRef, "error", err)
		d.writeErr(w, http.StatusBadGateway, "api_error", "upstream credential unavailable")
		return
	}

	outReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		strings.TrimSuffix(up.BaseURL, "/")+d.path, bytes.NewReader(outBody))
	if err != nil {
		d.writeErr(w, http.StatusBadGateway, "api_error", "building upstream request failed")
		return
	}
	copyHeaders(outReq.Header, r.Header)
	setUpstreamCredential(outReq, up, d, cred)
	if len(outBody) != len(body) {
		outReq.ContentLength = int64(len(outBody))
	}

	resp, err := s.httpClient().Do(outReq)
	if err != nil {
		s.log.Error("upstream request failed", "route", d.route, "error", err)
		d.writeErr(w, http.StatusBadGateway, "api_error", "upstream unreachable")
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		if hopByHop[http.CanonicalHeaderKey(k)] {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	audit.UpstreamStatus = resp.StatusCode
	var tokensIn, tokensOut int64
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		audit.Stream = true
		tokensIn, tokensOut, err = copyStreamWith(w, resp.Body, d.newScanner())
	} else {
		tokensIn, tokensOut, err = relayWithUsage(w, resp.Body, d.jsonUsage)
	}
	if err != nil {
		s.log.Warn("response relay interrupted", "error", err)
	}
	audit.LatencyMS = time.Since(start).Milliseconds()
	s.record(audit, tokensIn, tokensOut)
}

// setUpstreamCredential presents the resolved credential the way this upstream
// expects it. The route's dialect supplies the default so single-route configs
// written before auth existed keep working unchanged.
func setUpstreamCredential(req *http.Request, up config.Upstream, d dialect, cred string) {
	auth := up.Auth
	if auth == "" {
		auth = d.defaultAuth
	}
	if auth == config.AuthBearer {
		req.Header.Set("Authorization", "Bearer "+cred)
		return
	}
	req.Header.Set("x-api-key", cred)
}

// peekModel reads just the model field off an undecoded body, for attribution
// on paths that never parse the document. An unparseable body yields "".
func peekModel(body []byte) string {
	var doc struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return ""
	}
	return doc.Model
}

func anthropicJSONUsage(buf []byte) (tokensIn, tokensOut int64) {
	var parsed struct {
		Usage anthropicUsage `json:"usage"`
	}
	if json.Unmarshal(buf, &parsed) != nil {
		return 0, 0
	}
	return parsed.Usage.InputTokens, parsed.Usage.OutputTokens
}

func openAIJSONUsage(buf []byte) (tokensIn, tokensOut int64) {
	var parsed struct {
		Usage openAIUsage `json:"usage"`
	}
	if json.Unmarshal(buf, &parsed) != nil {
		return 0, 0
	}
	return parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens
}

// relayWithUsage copies a JSON response to the caller byte-faithfully while
// extracting usage token counts for the spine metrics. Bodies beyond 4 MB are
// relayed without parsing. The streaming equivalent lives in stream.go, which
// scans usage events as they go by.
func relayWithUsage(w http.ResponseWriter, body io.Reader, extract func([]byte) (int64, int64)) (tokensIn, tokensOut int64, err error) {
	buf, err := io.ReadAll(io.LimitReader(body, 4<<20))
	if err != nil {
		return 0, 0, err
	}
	if _, werr := w.Write(buf); werr != nil {
		return 0, 0, werr
	}
	// drain any remainder past the parse limit
	if _, derr := io.Copy(w, body); derr != nil {
		return 0, 0, derr
	}
	tokensIn, tokensOut = extract(buf)
	return tokensIn, tokensOut, nil
}

// copyHeaders forwards everything except hop-by-hop and the caller's own
// credential headers (the whole point is that the client key never reaches
// the vendor).
func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		ck := http.CanonicalHeaderKey(k)
		if hopByHop[ck] || clientAuthHeaders[ck] || ck == "Host" {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// upstreamTransport bounds everything that happens BEFORE the response body
// starts, and nothing after it.
//
// http.Client.Timeout is deliberately not set and must not be: it covers the
// body read too, so any value large enough for a long generation is useless as
// a hang detector, and any value small enough to detect a hang truncates a
// legitimate stream mid-token. ResponseHeaderTimeout is the right knob —
// a vendor that accepts the connection and then says nothing is caught in
// seconds, while an SSE response that has begun flowing may run as long as it
// likes. Cancellation still rides the request context, so a client that goes
// away tears the upstream call down with it.
var upstreamTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	ForceAttemptHTTP2:     true,
}

var upstreamClient = &http.Client{Transport: upstreamTransport}

func (s *Server) httpClient() *http.Client { return upstreamClient }

// writeVendorError renders an Anthropic-shaped error. Callers on that route
// parse this envelope, so its shape is part of the contract.
func writeVendorError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"type":"error","error":{"type":%q,"message":%q}}`+"\n", errType, msg)
}

// writeOpenAIError renders the same failure in the OpenAI error envelope, so
// an OpenAI-compatible SDK surfaces the gateway's own rejections as errors
// rather than as an unparseable body.
func writeOpenAIError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":{"message":%q,"type":%q,"param":null,"code":null}}`+"\n", msg, errType)
}
