package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

// requireClientKey authenticates the caller's gateway key (Bearer or
// x-api-key) against the resolved client-key set.
func (s *Server) requireClientKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-api-key")
		if key == "" {
			key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if _, ok := s.clientKeys[key]; !ok || key == "" {
			writeVendorError(w, http.StatusUnauthorized, "authentication_error", "invalid gateway key")
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

// handleMessages is the Anthropic Messages proxy route: read → detect →
// redact/block → swap credential → forward → return (byte-faithful when
// nothing was redacted).
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	up, ok := s.cfg.Upstreams["anthropic"]
	if !ok {
		writeVendorError(w, http.StatusBadGateway, "api_error", "no upstream configured for route anthropic")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		writeVendorError(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "body too large or unreadable")
		return
	}

	audit := RequestAudit{
		RequestID: r.Header.Get("X-Gateway-Request-Id"),
		Route:     "anthropic",
		Action:    actionPass,
		At:        time.Now().UTC(),
	}

	outBody := body
	action := s.currentAction()
	if action != actionPass {
		detectStart := time.Now()
		var doc map[string]any
		if err := json.Unmarshal(body, &doc); err != nil {
			writeVendorError(w, http.StatusBadRequest, "invalid_request_error", "body is not valid JSON")
			return
		}
		var redactions []Redaction
		var detErrs []string
		mutated := false
		for _, f := range walkAnthropicBody(doc) {
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
			writeVendorError(w, http.StatusServiceUnavailable, "api_error",
				"detector unavailable and GATEWAY_FAIL_MODE=closed")
			return
		}

		if len(redactions) > 0 {
			switch action {
			case actionBlock:
				audit.Action = actionBlock
				audit.LatencyMS = time.Since(start).Milliseconds()
				s.record(audit, 0, 0)
				writeVendorError(w, http.StatusBadRequest, "invalid_request_error",
					"request blocked by gateway policy: detected "+typeSummary(redactions))
				return
			case actionMask:
				audit.Action = actionMask
				if mutated {
					rewritten, err := json.Marshal(doc)
					if err != nil {
						writeVendorError(w, http.StatusInternalServerError, "api_error", "rewrite failed")
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
		s.log.Error("credential resolution failed", "route", "anthropic", "ref", up.CredentialRef, "error", err)
		writeVendorError(w, http.StatusBadGateway, "api_error", "upstream credential unavailable")
		return
	}

	outReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		strings.TrimSuffix(up.BaseURL, "/")+"/v1/messages", bytes.NewReader(outBody))
	if err != nil {
		writeVendorError(w, http.StatusBadGateway, "api_error", "building upstream request failed")
		return
	}
	copyHeaders(outReq.Header, r.Header)
	outReq.Header.Set("x-api-key", cred)
	if len(outBody) != len(body) {
		outReq.ContentLength = int64(len(outBody))
	}

	resp, err := s.httpClient().Do(outReq)
	if err != nil {
		s.log.Error("upstream request failed", "route", "anthropic", "error", err)
		writeVendorError(w, http.StatusBadGateway, "api_error", "upstream unreachable")
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
		err = copyStream(w, resp.Body)
	} else {
		tokensIn, tokensOut, err = relayWithUsage(w, resp.Body)
	}
	if err != nil {
		s.log.Warn("response relay interrupted", "error", err)
	}
	audit.LatencyMS = time.Since(start).Milliseconds()
	s.record(audit, tokensIn, tokensOut)
}

// relayWithUsage copies a JSON response to the caller byte-faithfully while
// extracting usage token counts for the spine metrics. Bodies beyond 4 MB are
// relayed without parsing. (SSE usage extraction is deferred — the
// message_delta events carry it; see TODO-gateway-deferred.md.)
func relayWithUsage(w http.ResponseWriter, body io.Reader) (tokensIn, tokensOut int64, err error) {
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
	var parsed struct {
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(buf, &parsed) == nil {
		tokensIn, tokensOut = parsed.Usage.InputTokens, parsed.Usage.OutputTokens
	}
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

func writeVendorError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"type":"error","error":{"type":%q,"message":%q}}`+"\n", errType, msg)
}
