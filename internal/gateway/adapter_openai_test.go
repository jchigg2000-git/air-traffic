package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"air-traffic/internal/gateway/config"
)

func walkOpenAIJSON(t *testing.T, body string) []textField {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return walkOpenAIBody(doc)
}

func TestOpenAIWalkCoversMessageToolAndPartFields(t *testing.T) {
	body := `{
	  "model": "meta-llama/Llama-3.3-70B-Instruct",
	  "tools": [{"type":"function","function":{"name":"lookup","description":"look up a member by SSN"}}],
	  "messages": [
	    {"role":"system","content":"you are helpful"},
	    {"role":"user","name":"jane roe","content":[
	      {"type":"text","text":"my ssn is 123-45-6789"},
	      {"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}
	    ]},
	    {"role":"assistant","content":null,"reasoning_content":"private reasoning","tool_calls":[
	      {"id":"tc1","type":"function","function":{"name":"lookup","arguments":"{\"ssn\":\"123-45-6789\"}"}}
	    ]},
	    {"role":"assistant","function_call":{"name":"legacy","arguments":"{\"email\":\"a@b.com\"}"}}
	  ]
	}`
	got := pathsOf(walkOpenAIJSON(t, body))

	want := map[string]string{
		"messages[0].content":                          "you are helpful",
		"messages[1].content[0].text":                  "my ssn is 123-45-6789",
		"messages[1].name":                             "jane roe",
		"messages[2].tool_calls[0].function.arguments": `{"ssn":"123-45-6789"}`,
		"messages[3].function_call.arguments":          `{"email":"a@b.com"}`,
		"tools[0].function.description":                "look up a member by SSN",
	}
	for path, text := range want {
		if got[path] != text {
			t.Errorf("field %s = %q, want %q", path, got[path], text)
		}
	}

	// Base64 pixels and provider-generated reasoning stay out, for the same
	// reasons the Anthropic adapter skips images and thinking blocks.
	for path := range got {
		if strings.Contains(path, "image_url") || strings.Contains(path, "reasoning") {
			t.Errorf("unscannable field walked: %s", path)
		}
	}
}

// The setters must rewrite the decoded document in place, and a masked
// arguments payload must still parse as JSON afterwards — [TYPE] carries no
// quote or backslash, which is why walking it as plain text is safe.
func TestOpenAIWalkSettersRewriteInPlace(t *testing.T) {
	body := `{"messages":[
	  {"role":"user","content":[{"type":"text","text":"ssn 123-45-6789"}]},
	  {"role":"assistant","tool_calls":[{"function":{"name":"n","arguments":"{\"email\":\"a@b.com\"}"}}]}
	]}`
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fields := walkOpenAIBody(doc)
	if len(fields) != 2 {
		t.Fatalf("fields = %d (%v)", len(fields), pathsOf(fields))
	}
	for _, f := range fields {
		f.set(strings.ReplaceAll(strings.ReplaceAll(f.text, "123-45-6789", "[SSN]"), "a@b.com", "[EMAIL]"))
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, leaked := range []string{"123-45-6789", "a@b.com"} {
		if strings.Contains(string(out), leaked) {
			t.Errorf("%q survived the rewrite: %s", leaked, out)
		}
	}

	var round map[string]any
	if err := json.Unmarshal(out, &round); err != nil {
		t.Fatalf("masked body no longer parses: %v", err)
	}
	args := round["messages"].([]any)[1].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"].(string)
	var inner map[string]any
	if err := json.Unmarshal([]byte(args), &inner); err != nil {
		t.Errorf("masked tool arguments are no longer valid JSON (%q): %v", args, err)
	}
}

// Malformed and hostile shapes must degrade to "nothing to walk", never panic.
func TestOpenAIWalkToleratesMalformedBodies(t *testing.T) {
	bodies := []string{
		`{}`,
		`{"messages":"nope","tools":7}`,
		`{"messages":[null,{"content":42},{"content":[null,{"type":"text"},{"type":"image_url"}]},{"tool_calls":"no"}]}`,
		`{"messages":[{"tool_calls":[{"function":"notanobject"}]}],"tools":[{"function":5}]}`,
	}
	for _, b := range bodies {
		if f := walkOpenAIJSON(t, b); len(f) != 0 {
			t.Errorf("body %s yielded %v", b, pathsOf(f))
		}
	}
}

const openAISSESample = `data: {"id":"c1","choices":[{"delta":{"content":"hi"},"index":0}],"usage":null}` + "\n\n" +
	`data: {"id":"c1","choices":[{"delta":{},"finish_reason":"stop","index":0}],"usage":null}` + "\n\n" +
	`data: {"id":"c1","choices":[],"usage":{"prompt_tokens":41,"completion_tokens":17,"total_tokens":58}}` + "\n\n" +
	"data: [DONE]\n\n"

func TestOpenAIStreamUsageScanner(t *testing.T) {
	rec := httptest.NewRecorder()
	in, out, err := copyStreamWith(rec, strings.NewReader(openAISSESample), newOpenAIUsageScanner())
	if err != nil {
		t.Fatalf("copyStreamWith: %v", err)
	}
	if in != 41 || out != 17 {
		t.Errorf("usage = (%d, %d), want (41, 17)", in, out)
	}
	if rec.Body.String() != openAISSESample {
		t.Errorf("stream was not relayed byte-faithfully:\n%q", rec.Body.String())
	}
}

// Split reads are the normal case on a real socket, and the terminal [DONE]
// sentinel must not be mistaken for a usage payload.
func TestOpenAIStreamUsageAcrossChunkBoundaries(t *testing.T) {
	in, out, err := copyStreamWith(httptest.NewRecorder(), iotestChunks(openAISSESample, 7), newOpenAIUsageScanner())
	if err != nil {
		t.Fatalf("copyStreamWith: %v", err)
	}
	if in != 41 || out != 17 {
		t.Errorf("usage across chunk boundaries = (%d, %d), want (41, 17)", in, out)
	}
}

// Without stream_options.include_usage the counts are legitimately absent.
// Reporting zero is correct; inventing a number would not be.
func TestOpenAIStreamWithoutUsageReportsZero(t *testing.T) {
	body := `data: {"id":"c1","choices":[{"delta":{"content":"hi"}}]}` + "\n\n" + "data: [DONE]\n\n"
	in, out, err := copyStreamWith(httptest.NewRecorder(), strings.NewReader(body), newOpenAIUsageScanner())
	if err != nil {
		t.Fatalf("copyStreamWith: %v", err)
	}
	if in != 0 || out != 0 {
		t.Errorf("usage = (%d, %d), want (0, 0)", in, out)
	}
}

func TestOpenAIJSONUsageExtraction(t *testing.T) {
	in, out := openAIJSONUsage([]byte(`{"id":"c1","usage":{"prompt_tokens":12,"completion_tokens":34}}`))
	if in != 12 || out != 34 {
		t.Errorf("usage = (%d, %d), want (12, 34)", in, out)
	}
	if in, out := openAIJSONUsage([]byte(`not json`)); in != 0 || out != 0 {
		t.Errorf("unparseable body should yield no usage, got (%d, %d)", in, out)
	}
}

// newDualRouteGateway wires both dialects at once, which is how the gateway
// actually runs when it fronts Anthropic and an OpenAI-compatible router
// together.
func newDualRouteGateway(t *testing.T, anthropicURL, openaiURL string) http.Handler {
	t.Helper()
	t.Setenv("GATEWAY_UPSTREAMS", fmt.Sprintf(
		`{"anthropic":{"base_url":%q,"credential_ref":"env:TEST_UPSTREAM_CRED"},`+
			`"openai":{"base_url":%q,"credential_ref":"env:TEST_OPENAI_CRED"}}`, anthropicURL, openaiURL))
	t.Setenv("TEST_UPSTREAM_CRED", testUpstreamCred)
	t.Setenv("TEST_OPENAI_CRED", "openai-secret-cred")
	t.Setenv("GATEWAY_CLIENT_KEYS_REF", "env:TEST_CLIENT_KEYS")
	t.Setenv("TEST_CLIENT_KEYS", testClientKey)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	gw, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return gw.Routes()
}

// The OpenAI route must reach <base_url>/chat/completions with the credential
// as a bearer token — the base URL already carries the /v1 segment, which is
// how every OpenAI-compatible endpoint is configured.
func TestChatCompletionsRoundTripAndBearerSwap(t *testing.T) {
	upstreamBody := `{"id":"c1","object":"chat.completion","usage":{"prompt_tokens":5,"completion_tokens":7}}`
	var seenPath, seenBearer, seenAPIKey, seenBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seenBody, seenPath = string(b), r.URL.Path
		seenBearer = r.Header.Get("Authorization")
		seenAPIKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, upstreamBody)
	}))
	defer upstream.Close()

	gw := httptest.NewServer(newDualRouteGateway(t, "http://unused.invalid", upstream.URL+"/v1"))
	defer gw.Close()

	reqBody := `{"model":"meta-llama/Llama-3.3-70B-Instruct","messages":[{"role":"user","content":"hello"}]}`
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+testClientKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)

	if seenPath != "/v1/chat/completions" {
		t.Errorf("upstream path = %q, want /v1/chat/completions", seenPath)
	}
	if seenBody != reqBody {
		t.Errorf("upstream body = %q, want byte-faithful %q", seenBody, reqBody)
	}
	if seenBearer != "Bearer openai-secret-cred" {
		t.Errorf("upstream Authorization = %q, want the resolved credential as a bearer token", seenBearer)
	}
	if seenAPIKey != "" {
		t.Errorf("x-api-key set on an OpenAI-dialect upstream: %q", seenAPIKey)
	}
	if strings.Contains(seenBearer, testClientKey) {
		t.Error("client key leaked upstream")
	}
	if string(got) != upstreamBody {
		t.Errorf("response = %q, want byte-faithful %q", got, upstreamBody)
	}
}

// A gateway-originated rejection must arrive in the caller's own dialect, or
// an OpenAI SDK cannot parse its own error.
func TestChatCompletionsRejectionUsesOpenAIErrorShape(t *testing.T) {
	gw := httptest.NewServer(newDualRouteGateway(t, "http://unused.invalid", "http://unused.invalid/v1"))
	defer gw.Close()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer wrong-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("error body is not JSON (%s): %v", body, err)
	}
	if env.Error.Message == "" || env.Error.Type == "" {
		t.Errorf("error envelope missing message/type: %s", body)
	}
}
