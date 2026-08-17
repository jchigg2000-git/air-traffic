package model

import "testing"

// The truth table behind PIVOT-1: applying "Healthcare" from the UI used to be
// an unconditional org-wide block, because nothing could set the attestation.
// These assertions pin the two halves of the fix — that the gate exists, and
// that the attestation is what opens it.
func TestGatewayAction(t *testing.T) {
	cases := []struct {
		name     string
		baseline Baseline
		attested bool
		want     string
	}{
		{"general_saas monitors", Baseline{PIIRedaction: "off", ZDR: "off"}, false, ActionDetect},
		{"fintech masks", Baseline{PIIRedaction: "on", ZDR: "where_native"}, false, ActionMask},
		{"healthcare unattested blocks", Baseline{PIIRedaction: "on+phi", ZDR: "enforced"}, false, ActionBlock},
		{"healthcare attested masks", Baseline{PIIRedaction: "on+phi", ZDR: "enforced"}, true, ActionMask},
		// gov_infra renders 🔒🔒🔒 like healthcare but never reaches the gated
		// branch, so it enforces strictly less than an unattested healthcare.
		{"gov_infra masks regardless", Baseline{PIIRedaction: "on", ZDR: "enforced"}, false, ActionMask},
		// The attestation must not loosen anything that was not gated on it.
		{"attestation does not weaken fintech", Baseline{PIIRedaction: "on", ZDR: "where_native"}, true, ActionMask},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := GatewayAction(tc.baseline, tc.attested); got != tc.want {
				t.Errorf("GatewayAction() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRequiresZDRAttestation(t *testing.T) {
	if !RequiresZDRAttestation(Baseline{PIIRedaction: "on+phi", ZDR: "enforced"}) {
		t.Error("healthcare must be marked as attestation-gated; the UI hides the checkbox otherwise")
	}
	if RequiresZDRAttestation(Baseline{PIIRedaction: "on", ZDR: "enforced"}) {
		t.Error("gov_infra is not attestation-gated; marking it so would offer a checkbox that changes nothing")
	}
}

// The attestation travels as a vendor flag, so an empty override map — exactly
// what the console sent before this fix — must read as not attested.
func TestZDRAttestedIn(t *testing.T) {
	if ZDRAttestedIn(nil) {
		t.Error("nil policy must not read as attested")
	}
	if ZDRAttestedIn(&Policy{Baseline: "healthcare"}) {
		t.Error("policy with no vendor overrides must not read as attested")
	}
	attested := &Policy{
		Baseline: "healthcare",
		Vendors:  map[string]map[string]any{ZDRAttestationVendor: {ZDRAttestationKey: true}},
	}
	if !ZDRAttestedIn(attested) {
		t.Error("policy carrying the attestation flag must read as attested")
	}
	// JSON round-trips can land a non-bool here; it must not panic or pass.
	wrongType := &Policy{Vendors: map[string]map[string]any{ZDRAttestationVendor: {ZDRAttestationKey: "true"}}}
	if ZDRAttestedIn(wrongType) {
		t.Error(`a string "true" must not satisfy the attestation`)
	}
}
