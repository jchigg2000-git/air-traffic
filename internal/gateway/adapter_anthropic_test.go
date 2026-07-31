package gateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func walkJSON(t *testing.T, body string) []textField {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return walkAnthropicBody(doc)
}

func pathsOf(fields []textField) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		out[f.path] = f.text
	}
	return out
}

func TestWalkCoversToolAndDocumentFields(t *testing.T) {
	body := `{
	  "system": "you are helpful",
	  "tools": [{"name":"lookup","description":"look up a member by SSN"}],
	  "messages": [
	    {"role":"assistant","content":[
	      {"type":"text","text":"calling the tool"},
	      {"type":"tool_use","id":"tu1","name":"lookup","input":{"ssn":"123-45-6789","nested":{"note":"call 555-0100"},"list":["a@b.com"],"count":3}}
	    ]},
	    {"role":"user","content":[
	      {"type":"tool_result","tool_use_id":"tu1","content":"member is Jane Roe"},
	      {"type":"tool_result","tool_use_id":"tu2","content":[{"type":"text","text":"dob 1970-01-01"}]},
	      {"type":"document","title":"chart","context":"visit notes","source":{"type":"text","media_type":"text/plain","data":"patient MRN 55512"}},
	      {"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}},
	      {"type":"thinking","thinking":"private reasoning","signature":"sig"}
	    ]}
	  ]
	}`
	got := pathsOf(walkJSON(t, body))

	want := map[string]string{
		"system":                                   "you are helpful",
		"tools[0].description":                     "look up a member by SSN",
		"messages[0].content[0].text":              "calling the tool",
		"messages[0].content[1].input.ssn":         "123-45-6789",
		"messages[0].content[1].input.nested.note": "call 555-0100",
		"messages[0].content[1].input.list[0]":     "a@b.com",
		"messages[1].content[0].content":           "member is Jane Roe",
		"messages[1].content[1].content[0].text":   "dob 1970-01-01",
		"messages[1].content[2].source.data":       "patient MRN 55512",
		"messages[1].content[2].title":             "chart",
		"messages[1].content[2].context":           "visit notes",
	}
	for path, text := range want {
		if got[path] != text {
			t.Errorf("field %s = %q, want %q", path, got[path], text)
		}
	}

	// Base64 pixels and signed thinking blocks stay out: scanning the first
	// finds nothing, rewriting the second breaks the signature.
	for path := range got {
		if strings.Contains(path, "content[3]") || strings.Contains(path, "content[4]") {
			t.Errorf("unscannable block walked: %s", path)
		}
	}
}

// The setters must rewrite the decoded document in place — that is what turns
// a detection into an actual redaction on the wire.
func TestWalkSettersRewriteNestedFields(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[
	  {"type":"tool_use","id":"t","name":"n","input":{"ssn":"123-45-6789","deep":[{"email":"a@b.com"}]}},
	  {"type":"tool_result","tool_use_id":"t","content":[{"type":"text","text":"secret"}]},
	  {"type":"document","source":{"type":"text","data":"mrn 55512"}}
	]}]}`
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fields := walkAnthropicBody(doc)
	if len(fields) != 4 {
		t.Fatalf("fields = %d (%v)", len(fields), pathsOf(fields))
	}
	for _, f := range fields {
		f.set("[REDACTED]")
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, leaked := range []string{"123-45-6789", "a@b.com", "secret", "mrn 55512"} {
		if strings.Contains(string(out), leaked) {
			t.Errorf("%q survived the rewrite: %s", leaked, out)
		}
	}
	if strings.Count(string(out), "[REDACTED]") != 4 {
		t.Errorf("rewrite count wrong: %s", out)
	}
}

// Malformed and hostile shapes must degrade to "nothing to walk", never panic.
func TestWalkToleratesMalformedBodies(t *testing.T) {
	bodies := []string{
		`{}`,
		`{"system":42,"messages":"nope","tools":7}`,
		`{"messages":[null,{"content":[null,{"type":"tool_result"},{"type":"document"},{"type":"tool_use","input":"notanobject"}]}]}`,
		`{"messages":[{"content":[{"type":"document","source":{"type":"base64","data":"AAAA"}}]}]}`,
	}
	for _, b := range bodies {
		if f := walkJSON(t, b); len(f) != 0 {
			t.Errorf("body %s yielded %v", b, pathsOf(f))
		}
	}
}

// Deeply nested tool input is bounded rather than recursing without limit.
func TestWalkBoundsRecursionDepth(t *testing.T) {
	deep := `"leaf"`
	for i := 0; i < 200; i++ {
		deep = `{"n":` + deep + `}`
	}
	body := `{"messages":[{"content":[{"type":"tool_use","input":` + deep + `}]}]}`
	fields := walkJSON(t, body)
	if len(fields) != 0 {
		t.Errorf("over-deep input should be dropped, got %v", pathsOf(fields))
	}
}
