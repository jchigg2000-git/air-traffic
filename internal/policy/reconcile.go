package policy

import (
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
)

// Apply resolves the policy (baseline ⊕ overrides), reconciles every adapter
// capability across the vendor_native and env_managed surfaces, records audit
// events, persists the policy, and returns the coverage report.
// gatewayStaleAfter mirrors the server's window: 3× the default 15s
// enforcement-heartbeat interval.
const gatewayStaleAfter = 45 * time.Second

func Apply(st *store.Store, p model.Policy) model.CoverageReport {
	base, _ := BaselineByID(p.Baseline)
	rep := model.CoverageReport{Baseline: p.Baseline, AppliedAt: time.Now().UTC(), Summary: map[string]int{}}

	// proxy_enforced flips from label to truth only while a gateway heartbeat
	// listing the capability is fresh (the honesty model, build plan §1).
	enforcedBy := func(vendor, capKey string) (string, bool) {
		id, fresh, _ := st.GatewayEnforcement(vendor, capKey, gatewayStaleAfter)
		return id, fresh
	}

	for _, a := range st.ListAdapters() {
		baaExcluded := base.BAAOnly && !a.BAASigned
		for _, c := range a.Capabilities {
			row := model.CoverageRow{
				Vendor: a.ID, Capability: c.Key, Plane: c.Plane,
				Disposition: c.Disposition, Enforcement: c.Enforcement,
			}
			if baaExcluded {
				row.Outcome = "excluded_no_baa"
				row.Signal = model.Signal{Disposition: c.Disposition, Code: "not_supported", Message: "vendor not BAA-signed; excluded by " + base.Name + " baseline"}
				rep.Rows = append(rep.Rows, row)
				rep.Summary[row.Outcome]++
				continue
			}
			row.Outcome, row.Signal = classify(a, c, enforcedBy)
			rep.Rows = append(rep.Rows, row)
			rep.Summary[row.Outcome]++

			if row.Outcome == "applied_native" || row.Outcome == "applied_env" || row.Outcome == "applied_proxy" {
				st.AddAudit(model.AuditEvent{
					Actor: "air-traffic:policy-engine", Action: "policy.apply." + c.Key,
					Resource: a.Vendor, Plane: c.Plane, Vendor: a.ID, ControlSurface: c.Disposition,
					After: map[string]any{"baseline": p.Baseline, "outcome": row.Outcome}, RequestID: model.NewUUID(),
				})
			}
		}
	}

	p.AppliedAt = rep.AppliedAt
	st.SetPolicy(p)
	st.RecordActivity("policy", "Applied baseline "+p.Baseline, "")
	st.AddAudit(model.AuditEvent{
		Actor: "air-traffic:admin", Action: "baseline.apply", Resource: "organization",
		Plane: model.PlaneDataPolicy, Vendor: "all", ControlSurface: model.DispVendorNative,
		After: map[string]any{"baseline": p.Baseline, "summary": rep.Summary}, RequestID: model.NewUUID(),
	})
	return rep
}

func classify(a model.Adapter, c model.Capability, enforcedBy func(vendor, capKey string) (string, bool)) (string, model.Signal) {
	switch c.Disposition {
	case model.DispVendorNative:
		return "applied_native", model.Signal{Disposition: c.Disposition, Code: "success", Message: "driven via vendor admin API (" + c.Mechanism + ")"}
	case model.DispEnvManaged:
		return "applied_env", model.Signal{Disposition: c.Disposition, Code: "success", Message: "managed config rendered + distributed (" + string(c.Enforcement) + ")"}
	case model.DispProxyEnforced:
		if id, ok := enforcedBy(a.ID, c.Key); ok {
			return "applied_proxy", model.Signal{Disposition: c.Disposition, Code: "success", Message: "enforced inline by gateway " + id}
		}
		return "proxy_needed", model.Signal{Disposition: c.Disposition, Code: "proxy_required", Message: "requires the optional gateway (off by default); behaves as monitor_only"}
	case model.DispMonitorOnly:
		return "monitored", model.Signal{Disposition: c.Disposition, Code: "success", Message: "verified + alerting; no set mechanism"}
	case model.DispUnverified:
		return "unverified", model.Signal{Disposition: c.Disposition, Code: "unverified", Message: "could not confirm from live vendor docs"}
	default:
		return "unsupported", model.Signal{Disposition: c.Disposition, Code: "not_supported", Message: "vendor does not support this control"}
	}
}
