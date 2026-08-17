package model

// The baseline → gateway-action derivation, and its one honest complication:
// the healthcare baseline is BLOCK until ZDR coverage is attested.
//
// It lives here, in the shared contract package, for the same reason the
// Presidio vocabulary does (presidio.go): two binaries must agree on it or
// they diverge. The gateway enforces it (internal/gateway/spine_pull.go) and
// the control plane has to be able to answer "what will this baseline do?"
// for the SPA, which cannot link gateway packages (G0 dependency isolation,
// enforced by cmd/air-traffic-server's depisolation test).
//
// Before this function existed the derivation lived only in the gateway, so
// nothing upstream of it could preview the outcome — and the Rigor Console
// shipped a primary CTA whose effect was unknowable from the page it sat on.
// One implementation means the preview and the enforcement cannot drift.

// Gateway action values — the wire strings the gateway enforces on.
const (
	ActionMask   = "mask"
	ActionBlock  = "block"
	ActionDetect = "detect"
)

// The policy path the ZDR attestation travels on. Named constants because
// three places have to agree: the SPA that writes the flag, the control plane
// that previews it, and the gateway's policy pull that reads it. It is carried
// as a vendor flag rather than a top-level field because ZDR is a property of
// a vendor contract.
const (
	ZDRAttestationVendor = "anthropic"
	ZDRAttestationKey    = "zdr_attested"
)

// GatewayAction maps a baseline to the redaction action the gateway will
// enforce (build plan G7):
//
//	pii_redaction off       → detect (log-only; monitoring, not enforcement)
//	pii_redaction on        → mask
//	pii_redaction on+phi    → block until ZDR coverage is attested (the
//	                          pre-coverage gate, design §15), then mask
//
// zdrAttested is the operator's assertion that the in-scope vendor contracts
// actually carry zero-data-retention coverage. It is a claim about paperwork
// the software cannot verify, which is why it is an explicit input rather
// than something inferred.
func GatewayAction(b Baseline, zdrAttested bool) string {
	switch b.PIIRedaction {
	case "off":
		return ActionDetect
	case "on+phi":
		if RequiresZDRAttestation(b) && !zdrAttested {
			return ActionBlock
		}
		return ActionMask
	default: // "on"
		return ActionMask
	}
}

// RequiresZDRAttestation reports whether this baseline's action depends on the
// attestation — i.e. whether applying it unattested is a total block.
//
// Worth stating because the UI's lock ramp implies otherwise: healthcare
// (on+phi) is gated; gov_infra is NOT, because its pii_redaction is plain "on"
// and never reaches the gated branch. gov_infra renders as joint-strictest and
// enforces strictly less than an unattested healthcare does.
func RequiresZDRAttestation(b Baseline) bool {
	return b.PIIRedaction == "on+phi" && b.ZDR == "enforced"
}

// ZDRAttestedIn reads the attestation out of an applied policy.
func ZDRAttestedIn(p *Policy) bool {
	if p == nil {
		return false
	}
	v, ok := p.Vendors[ZDRAttestationVendor]
	if !ok {
		return false
	}
	attested, _ := v[ZDRAttestationKey].(bool)
	return attested
}
