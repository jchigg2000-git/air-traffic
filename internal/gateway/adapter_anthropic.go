package gateway

import "fmt"

// textField is one located text-bearing field of a request body, with a
// setter that rewrites it in place in the decoded document.
type textField struct {
	path string
	text string
	set  func(string)
}

// walkAnthropicBody locates the known text-bearing fields of an Anthropic
// Messages request: system (string | text blocks) and messages[].content
// (string | text blocks). Multimodal and tool-call payloads are a known gap
// (build-plan §7 adapter-drift risk) tracked in TODO-gateway-deferred.md.
func walkAnthropicBody(doc map[string]any) []textField {
	var fields []textField
	switch sys := doc["system"].(type) {
	case string:
		fields = append(fields, textField{path: "system", text: sys,
			set: func(v string) { doc["system"] = v }})
	case []any:
		fields = append(fields, walkTextBlocks(sys, "system")...)
	}
	msgs, _ := doc["messages"].([]any)
	for i, m := range msgs {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		path := fmt.Sprintf("messages[%d].content", i)
		switch c := mm["content"].(type) {
		case string:
			fields = append(fields, textField{path: path, text: c,
				set: func(v string) { mm["content"] = v }})
		case []any:
			fields = append(fields, walkTextBlocks(c, path)...)
		}
	}
	return fields
}

func walkTextBlocks(blocks []any, prefix string) []textField {
	var fields []textField
	for j, b := range blocks {
		bm, ok := b.(map[string]any)
		if !ok || bm["type"] != "text" {
			continue
		}
		t, ok := bm["text"].(string)
		if !ok {
			continue
		}
		fields = append(fields, textField{path: fmt.Sprintf("%s[%d].text", prefix, j), text: t,
			set: func(v string) { bm["text"] = v }})
	}
	return fields
}
