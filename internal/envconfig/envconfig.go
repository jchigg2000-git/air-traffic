// Package envconfig renders env_managed configuration artifacts (Claude Code
// managed-settings.json, GitHub branch protection, VS Code, Cursor), records the
// synthetic "push", and reads effective state back for drift detection.
package envconfig

import (
	"time"

	"air-traffic/internal/model"
)

// Render produces a byte-faithful ManagedArtifact for the given platform from policy intent.
func Render(platform, identifier string, agentic map[string]any, enf model.Enforcement) model.ManagedArtifact {
	art := model.ManagedArtifact{
		Platform:    platform,
		Identifier:  identifier,
		Enforcement: enf,
		RenderedAt:  time.Now().UTC(),
	}
	switch platform {
	case "claude_code":
		allow, deny := mcpLists(agentic)
		art.Artifact = map[string]any{
			"permissions": map[string]any{
				"allow": []any{"Read", "Edit", "Bash(git diff:*)"},
				"deny":  []any{"Bash(rm -rf:*)"},
			},
			"allowedMcpServers":               allow,
			"deniedMcpServers":                deny,
			"allowManagedMcpServersOnly":      true,
			"allowManagedHooksOnly":           true,
			"allowManagedPermissionRulesOnly": true,
			"disableSkillShellExecution":      false,
			"hooks": map[string]any{
				"PreToolUse": []any{map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "secret-scan"}}}},
			},
		}
		art.DistributionURL = "mdm://jamf/profiles/claude-code-managed-settings"
	case "github":
		art.Artifact = map[string]any{
			"required_status_checks": map[string]any{"strict": true, "contexts": []any{"tests", "coverage-90"}},
			"enforce_admins":         true,
			"required_pull_request_reviews": map[string]any{
				"required_approving_review_count": 1,
				"require_code_owner_reviews":      true,
			},
			"restrictions": nil,
		}
		art.DistributionURL = "https://api.github.com/repos/acme/payments/branches/main/protection"
	case "vs_code":
		art.Artifact = map[string]any{
			"ChatMCP":           "disabled",
			"ChatAgentMode":     "restricted",
			"AllowedExtensions": []any{"github.copilot", "anthropic.claude-code"},
		}
		art.DistributionURL = "intune://policy/vscode-enterprise"
	case "cursor":
		art.Artifact = map[string]any{
			"mcp_allow_list": []any{"filesystem", "git"},
			"zdr":            true,
		}
		art.DistributionURL = "https://cursor.com/admin/api/policy"
	default:
		art.Artifact = map[string]any{}
	}
	return art
}

func mcpLists(agentic map[string]any) (allow []any, deny []any) {
	allow = []any{"filesystem", "git", "internal-db-readonly"}
	deny = []any{"*"}
	if agentic == nil {
		return
	}
	if cc, ok := agentic["claude_code"].(map[string]any); ok {
		if mcp, ok := cc["mcp"].(map[string]any); ok {
			if a, ok := mcp["allow"].([]any); ok && len(a) > 0 {
				allow = a
			}
			if d, ok := mcp["deny"].([]any); ok && len(d) > 0 {
				deny = d
			}
		}
	}
	return
}

// ReadState returns the synthetic effective state of a managed surface. A small
// fraction of reads synthesize a Tier-C user override to exercise drift detection.
func ReadState(platform, identifier string, artifact model.ManagedArtifact, overridden bool) model.EnvState {
	st := model.EnvState{
		Platform:       platform,
		Identifier:     identifier,
		ActualSettings: artifact.Artifact,
		Source:         "locked",
		ReadAt:         time.Now().UTC(),
	}
	switch artifact.Enforcement {
	case model.EnforcementSeedOnly:
		st.Source = "default"
		if overridden {
			st.Source = "user_override"
			st.DriftDetected = true
			st.DriftMessage = "developer overrode seed-only config on an unmanaged device"
		}
	case model.EnforcementMDMLocked:
		st.Source = "locked"
		if overridden {
			st.Source = "user_override"
			st.DriftDetected = true
			st.DriftMessage = "device not MDM-enrolled; managed settings not applied"
		}
	}
	return st
}
