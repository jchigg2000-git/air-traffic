package envconfig

import (
	"reflect"
	"testing"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

var (
	defaultAllow = []any{"filesystem", "git", "internal-db-readonly"}
	defaultDeny  = []any{"*"}
)

func TestRenderClaudeCodeMCPLists(t *testing.T) {
	cases := []struct {
		name      string
		agentic   map[string]any
		wantAllow []any
		wantDeny  []any
	}{
		{
			name: "explicit allow and deny lists are used",
			agentic: map[string]any{"claude_code": map[string]any{"mcp": map[string]any{
				"allow": []any{"github"},
				"deny":  []any{"shell", "browser"},
			}}},
			wantAllow: []any{"github"},
			wantDeny:  []any{"shell", "browser"},
		},
		{name: "nil agentic falls back to defaults", agentic: nil, wantAllow: defaultAllow, wantDeny: defaultDeny},
		{name: "missing claude_code falls back", agentic: map[string]any{"cursor": map[string]any{}}, wantAllow: defaultAllow, wantDeny: defaultDeny},
		{
			name:      "missing mcp falls back",
			agentic:   map[string]any{"claude_code": map[string]any{}},
			wantAllow: defaultAllow,
			wantDeny:  defaultDeny,
		},
		{
			name: "non-list values fall back",
			agentic: map[string]any{"claude_code": map[string]any{"mcp": map[string]any{
				"allow": "github",
				"deny":  []string{"shell"}, // typed slice, not []any
			}}},
			wantAllow: defaultAllow,
			wantDeny:  defaultDeny,
		},
		{
			name: "empty lists fall back",
			agentic: map[string]any{"claude_code": map[string]any{"mcp": map[string]any{
				"allow": []any{},
				"deny":  []any{},
			}}},
			wantAllow: defaultAllow,
			wantDeny:  defaultDeny,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			art := Render("claude_code", "acme", tc.agentic, model.EnforcementMDMLocked)
			if art.Platform != "claude_code" || art.Identifier != "acme" || art.Enforcement != model.EnforcementMDMLocked {
				t.Errorf("artifact header = %q/%q/%q", art.Platform, art.Identifier, art.Enforcement)
			}
			if art.DistributionURL == "" || art.RenderedAt.IsZero() {
				t.Errorf("DistributionURL/RenderedAt not set: %q %v", art.DistributionURL, art.RenderedAt)
			}
			if got := art.Artifact["allowedMcpServers"]; !reflect.DeepEqual(got, tc.wantAllow) {
				t.Errorf("allowedMcpServers = %#v, want %#v", got, tc.wantAllow)
			}
			if got := art.Artifact["deniedMcpServers"]; !reflect.DeepEqual(got, tc.wantDeny) {
				t.Errorf("deniedMcpServers = %#v, want %#v", got, tc.wantDeny)
			}
		})
	}
}

func TestRenderUnknownPlatformIsEmpty(t *testing.T) {
	art := Render("emacs", "x", nil, model.EnforcementSeedOnly)
	if art.Artifact == nil || len(art.Artifact) != 0 || art.DistributionURL != "" {
		t.Fatalf("unknown platform artifact = %#v url=%q, want empty map and no URL", art.Artifact, art.DistributionURL)
	}
}

func TestReadState(t *testing.T) {
	cases := []struct {
		name       string
		enf        model.Enforcement
		overridden bool
		wantSource string
		wantDrift  bool
	}{
		{"seed_only clean", model.EnforcementSeedOnly, false, "default", false},
		{"seed_only overridden reports drift", model.EnforcementSeedOnly, true, "user_override", true},
		{"mdm_locked clean", model.EnforcementMDMLocked, false, "locked", false},
		{"mdm_locked overridden reports drift", model.EnforcementMDMLocked, true, "user_override", true},
		{"server_side ignores override", model.EnforcementServerSide, true, "locked", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			art := Render("claude_code", "acme", nil, tc.enf)
			st := ReadState("claude_code", "acme", art, tc.overridden)
			if st.Platform != "claude_code" || st.Identifier != "acme" || st.ReadAt.IsZero() {
				t.Errorf("state header = %q/%q read_at=%v", st.Platform, st.Identifier, st.ReadAt)
			}
			if !reflect.DeepEqual(st.ActualSettings, art.Artifact) {
				t.Errorf("ActualSettings does not echo the rendered artifact")
			}
			if st.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", st.Source, tc.wantSource)
			}
			if st.DriftDetected != tc.wantDrift {
				t.Errorf("DriftDetected = %v, want %v", st.DriftDetected, tc.wantDrift)
			}
			if tc.wantDrift && st.DriftMessage == "" {
				t.Errorf("drift reported without a DriftMessage")
			}
			if !tc.wantDrift && st.DriftMessage != "" {
				t.Errorf("DriftMessage = %q with no drift", st.DriftMessage)
			}
		})
	}
}
