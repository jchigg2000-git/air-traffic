package redact

import (
	"reflect"
	"testing"
)

func TestHeadersMasksOnlySensitiveKeysAndDoesNotAlias(t *testing.T) {
	in := map[string][]string{
		"Authorization": {"Bearer x"},
		"Accept":        {"*/*"},
	}
	got := Headers(in)
	want := map[string][]string{
		"Authorization": {placeholder},
		"Accept":        {"*/*"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Headers = %v, want %v", got, want)
	}
	// Mutating the copy must not reach the caller's slice.
	got["Accept"][0] = "mutated"
	if in["Accept"][0] != "*/*" {
		t.Fatalf("Headers aliased the input slice: in[Accept] = %q", in["Accept"][0])
	}
}

func TestQueryMasksCaseInsensitively(t *testing.T) {
	cases := []struct {
		name string
		in   map[string][]string
		want map[string][]string
	}{
		{"api_key", map[string][]string{"api_key": {"sk-1"}}, map[string][]string{"api_key": {placeholder}}},
		{"X-Goog-Api-Key", map[string][]string{"X-Goog-Api-Key": {"g"}}, map[string][]string{"X-Goog-Api-Key": {placeholder}}},
		{"AUTHORIZATION", map[string][]string{"AUTHORIZATION": {"a"}}, map[string][]string{"AUTHORIZATION": {placeholder}}},
		{"benign", map[string][]string{"model": {"gpt"}}, map[string][]string{"model": {"gpt"}}},
		{"nil", nil, map[string][]string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Query(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Query(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestHasPlaintextSecretKey(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want bool
	}{
		{"secret_ref allowed", map[string]any{"secret_ref": "env:X"}, false},
		{"api_key rejected", map[string]any{"api_key": "sk-1"}, true},
		{"nested token rejected", map[string]any{"auth": map[string]any{"token": "t"}}, true},
		{"benign", map[string]any{"name": "openai", "kind": "vendor"}, false},
		{"empty", map[string]any{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasPlaintextSecretKey(c.body); got != c.want {
				t.Errorf("HasPlaintextSecretKey(%v) = %v, want %v", c.body, got, c.want)
			}
		})
	}
}

func TestSecretRefShapes(t *testing.T) {
	for _, ref := range []string{"env:OPENAI_API_KEY", "vault://kv/openai", "kms:arn:aws:kms:us-east-1:123456789012:key/abc"} {
		if !IsSecretRef(ref) || LooksLikeRawSecret(ref) {
			t.Errorf("%q should be a reference", ref)
		}
	}
	for _, raw := range []string{"sk-ant-api03-x", "ghp_x", "AKIAX", "xoxb-1", "ya29.x"} {
		if !LooksLikeRawSecret(raw) || IsSecretRef(raw) {
			t.Errorf("%q should look like a raw credential", raw)
		}
	}
	for _, bad := range []string{"", "plaintext", "env:MY KEY", "file:/etc/key"} {
		if IsSecretRef(bad) {
			t.Errorf("%q should not be a reference", bad)
		}
	}
}
