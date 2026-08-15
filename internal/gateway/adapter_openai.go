package gateway

import "fmt"

// walkOpenAIBody locates the text-bearing fields of an OpenAI-compatible
// chat-completions request — the dialect the Hugging Face router, and most
// "OpenAI-compatible" endpoints, actually speak:
//
//	messages[].content                       string | parts, where a part may
//	                                         be text (image_url/input_audio are
//	                                         skipped, see below)
//	messages[].name                          participant label, caller-authored
//	messages[].tool_calls[].function.arguments   JSON-encoded call arguments
//	messages[].function_call.arguments       the deprecated single-call form
//	tools[].function.description             free text authored per request
//
// The system prompt needs no special case here: unlike Anthropic, OpenAI
// carries it as a messages[] entry with role "system", so the messages walk
// already covers it.
//
// Deliberately NOT walked, for the same reason as the Anthropic adapter —
// redacting them would be a lie rather than a gap (build-plan §7 adapter
// drift):
//
//	image_url / input_audio parts — PII inside pixels or audio needs OCR or
//	  transcription; a regex over base64 finds nothing, so scanning it would
//	  overstate enforcement. Blocking those parts is a policy question (G4).
//	reasoning / reasoning_content — provider-generated chain-of-thought echoed
//	  back on later turns. Treated like Anthropic's thinking blocks: rewriting
//	  content the provider expects to round-trip verbatim risks a rejection
//	  upstream, and we would be redacting our own model's output rather than
//	  the caller's input.
//
// Arguments strings are walked as plain text on purpose. Masking rewrites a
// span to [TYPE], which contains no quote or backslash, so a JSON-encoded
// arguments payload stays parseable after redaction.
func walkOpenAIBody(doc map[string]any) []textField {
	var fields []textField

	msgs, _ := doc["messages"].([]any)
	for i, m := range msgs {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		path := fmt.Sprintf("messages[%d]", i)
		switch c := mm["content"].(type) {
		case string:
			fields = append(fields, textField{path: path + ".content", text: c,
				set: func(v string) { mm["content"] = v }})
		case []any:
			fields = append(fields, walkOpenAIParts(c, path+".content")...)
		}
		fields = append(fields, stringField(mm, "name", path+".name")...)

		// The model's own output round-trips back through here, so a tool call
		// it emitted is exactly where PII re-enters the body.
		calls, _ := mm["tool_calls"].([]any)
		for j, tc := range calls {
			tcm, ok := tc.(map[string]any)
			if !ok {
				continue
			}
			if fn, ok := tcm["function"].(map[string]any); ok {
				fields = append(fields, stringField(fn, "arguments",
					fmt.Sprintf("%s.tool_calls[%d].function.arguments", path, j))...)
			}
		}
		if fn, ok := mm["function_call"].(map[string]any); ok {
			fields = append(fields, stringField(fn, "arguments", path+".function_call.arguments")...)
		}
	}

	tools, _ := doc["tools"].([]any)
	for i, t := range tools {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		if fn, ok := tm["function"].(map[string]any); ok {
			fields = append(fields, stringField(fn, "description",
				fmt.Sprintf("tools[%d].function.description", i))...)
		}
	}
	return fields
}

// walkOpenAIParts walks a multi-part content array, taking only text parts.
func walkOpenAIParts(parts []any, prefix string) []textField {
	var fields []textField
	for j, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if pm["type"] != "text" {
			continue
		}
		fields = append(fields, stringField(pm, "text", fmt.Sprintf("%s[%d].text", prefix, j))...)
	}
	return fields
}
