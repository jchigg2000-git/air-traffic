package gateway

import "fmt"

// textField is one located text-bearing field of a request body, with a
// setter that rewrites it in place in the decoded document.
type textField struct {
	path string
	text string
	set  func(string)
}

// maxWalkDepth bounds recursion into tool_use inputs — an attacker-supplied
// body must not be able to blow the stack via nesting alone.
const maxWalkDepth = 32

// walkAnthropicBody locates the text-bearing fields of an Anthropic Messages
// request:
//
//	system                        string | text blocks
//	messages[].content            string | blocks, where a block may be
//	                              text, tool_result (string | nested blocks),
//	                              tool_use (arbitrary JSON input), or
//	                              document (source.data when text/plain, plus
//	                              title/context)
//	tools[].description           free text authored per request
//
// Deliberately NOT walked, because redacting them would be a lie rather than a
// gap (build-plan §7 adapter drift):
//
//	image / document base64 payloads — PII inside pixels or a PDF needs OCR;
//	  a regex over base64 finds nothing, so pretending to scan it would
//	  overstate enforcement. Blocking those blocks is a policy question (G4).
//	thinking / redacted_thinking blocks — carry a server-issued signature;
//	  rewriting the text invalidates it and the request is rejected upstream.
func walkAnthropicBody(doc map[string]any) []textField {
	var fields []textField
	switch sys := doc["system"].(type) {
	case string:
		fields = append(fields, textField{path: "system", text: sys,
			set: func(v string) { doc["system"] = v }})
	case []any:
		fields = append(fields, walkContentBlocks(sys, "system")...)
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
			fields = append(fields, walkContentBlocks(c, path)...)
		}
	}

	tools, _ := doc["tools"].([]any)
	for i, t := range tools {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if d, ok := tm["description"].(string); ok {
			fields = append(fields, textField{path: fmt.Sprintf("tools[%d].description", i), text: d,
				set: func(v string) { tm["description"] = v }})
		}
	}
	return fields
}

// walkContentBlocks walks one content array, dispatching per block type.
func walkContentBlocks(blocks []any, prefix string) []textField {
	var fields []textField
	for j, b := range blocks {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		path := fmt.Sprintf("%s[%d]", prefix, j)
		switch bm["type"] {
		case "text":
			fields = append(fields, stringField(bm, "text", path+".text")...)

		case "tool_result":
			// The model's own output round-trips back through here, so a tool
			// that returned a record is exactly where PII re-enters the body.
			switch c := bm["content"].(type) {
			case string:
				fields = append(fields, textField{path: path + ".content", text: c,
					set: func(v string) { bm["content"] = v }})
			case []any:
				fields = append(fields, walkContentBlocks(c, path+".content")...)
			}

		case "tool_use":
			// input is caller-shaped JSON: walk every string leaf.
			if in, ok := bm["input"].(map[string]any); ok {
				fields = append(fields, walkJSONStrings(in, path+".input", 0)...)
			}

		case "document":
			// Only a text source is scannable; base64 PDFs are not (see above).
			if src, ok := bm["source"].(map[string]any); ok && src["type"] == "text" {
				fields = append(fields, stringField(src, "data", path+".source.data")...)
			}
			fields = append(fields, stringField(bm, "title", path+".title")...)
			fields = append(fields, stringField(bm, "context", path+".context")...)
		}
	}
	return fields
}

// walkJSONStrings yields every string leaf of an arbitrary JSON value, with a
// setter that writes it back into its container.
func walkJSONStrings(v any, path string, depth int) []textField {
	if depth > maxWalkDepth {
		return nil
	}
	var fields []textField
	switch node := v.(type) {
	case map[string]any:
		for k := range node {
			child := fmt.Sprintf("%s.%s", path, k)
			switch cv := node[k].(type) {
			case string:
				fields = append(fields, textField{path: child, text: cv,
					set: func(nv string) { node[k] = nv }})
			default:
				fields = append(fields, walkJSONStrings(cv, child, depth+1)...)
			}
		}
	case []any:
		for i := range node {
			child := fmt.Sprintf("%s[%d]", path, i)
			switch cv := node[i].(type) {
			case string:
				fields = append(fields, textField{path: child, text: cv,
					set: func(nv string) { node[i] = nv }})
			default:
				fields = append(fields, walkJSONStrings(cv, child, depth+1)...)
			}
		}
	}
	return fields
}

// stringField yields one field for obj[key] when it holds a string.
func stringField(obj map[string]any, key, path string) []textField {
	s, ok := obj[key].(string)
	if !ok {
		return nil
	}
	return []textField{{path: path, text: s, set: func(v string) { obj[key] = v }}}
}
