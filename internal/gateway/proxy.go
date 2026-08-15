package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"air-traffic/internal/gateway/config"
	"air-traffic/internal/gateway/redact"
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
// x-api-key) against the resolved client-key set. The rejection is written in
// the caller's own dialect, so a client SDK can parse the 401 it gets back.
func (s *Server) requireClientKey(writeErr func(http.ResponseWriter, int, string, string), next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-api-key")
		if key == "" {
			key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if _, ok := s.clientKeys[key]; !ok || key == "" {
			writeErr(w, http.StatusUnauthorized, "authentication_error", "invalid gateway key")
			return
		}
		next(w, r)
	}
}

// action values the pipeline can run under. mask/block come from config or
// policy; detect (log-only) and pass exist for policy-driven monitor routes.
const (
	actionMask   = "mask"
	actionBlock  = "block"
	actionDetect = "detect"
	actionPass   = "pass"
)

// currentAction resolves the effective redaction action: the static config
// value, or the policy-derived one when per_policy (masking until the first
// policy pull lands — safe default).
func (s *Server) currentAction() string {
	if s.cfg.RedactAction != "per_policy" {
		return s.cfg.RedactAction
	}
	if v := s.policyAction.Load(); v != nil {
		return v.(string)
	}
	return actionMask
}

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

	audit := RequestAudit{
		RequestID: r.Header.Get("X-Gateway-Request-Id"),
		Route:     d.route,
		Action:    actionPass,
		At:        time.Now().UTC(),
	}

	outBody := body
	action := s.currentAction()
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

func (s *Server) httpClient() *http.Client {
	// No global timeout: streaming responses legitimately run long. Connection
	// establishment uses the transport's defaults; cancellation rides the
	// request context.
	return http.DefaultClient
}

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
