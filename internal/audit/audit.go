// Package audit normalizes the cross-vendor audit stream and shapes a SIEM export
// (OTel GenAI semconv flavored). The raw events live in the store; this package
// provides the export transform.
package audit

import "air-traffic/internal/model"

// ToSIEM flattens normalized audit events into a SIEM-ingestable record set
// (Splunk/Elastic/Datadog style: flat keys, ISO timestamps).
func ToSIEM(events []model.AuditEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, e := range events {
		out = append(out, map[string]any{
			"@timestamp":          e.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			"event.id":            e.ID,
			"event.action":        e.Action,
			"event.category":      string(e.Plane),
			"user.name":           e.Actor,
			"cloud.provider":      e.Vendor,
			"gen_ai.system":       e.Vendor,
			"airtraffic.surface":  string(e.ControlSurface),
			"airtraffic.resource": e.Resource,
			"airtraffic.before":   e.Before,
			"airtraffic.after":    e.After,
			"trace.id":            e.RequestID,
		})
	}
	return out
}

// Seed returns a handful of starter audit events so the stream is non-empty at boot.
func Seed() []model.AuditEvent {
	return []model.AuditEvent{
		{Action: "model_access.set", Actor: "ada-admin", Resource: "OpenAI", Plane: model.PlaneDeveloperWorkflow, Vendor: "openai", ControlSurface: model.DispVendorNative,
			Before: map[string]any{"mode": "open"}, After: map[string]any{"mode": "allow_list", "models": []string{"gpt-4o", "o3-mini"}}, RequestID: model.NewUUID()},
		{Action: "mcp.policy.set", Actor: "air-traffic:config-distributor", Resource: "claude-code:internal-db-readonly", Plane: model.PlaneDeveloperWorkflow, Vendor: "anthropic", ControlSurface: model.DispEnvManaged,
			Before: map[string]any{"allow": []string{"filesystem"}}, After: map[string]any{"allow": []string{"filesystem", "internal-db-readonly"}}, RequestID: model.NewUUID()},
		{Action: "guardrail.attach", Actor: "sec-eng", Resource: "BEDROCK_POLICY", Plane: model.PlaneDataPolicy, Vendor: "bedrock", ControlSurface: model.DispVendorNative,
			After: map[string]any{"guardrail_id": "g-12345abc", "apply_cross_account": true}, RequestID: model.NewUUID()},
	}
}
