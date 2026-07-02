package model

// Shared Presidio vocabulary. Two consumers must agree on it or the
// flywheel's evidence diverges from enforcement: the gateway's detect
// adapter (internal/gateway/detect/presidio.go) and the harness's raw-score
// probe (internal/harness/probe.go). It lives here because the control-plane
// binary must never link gateway packages (G0 dependency isolation, enforced
// by cmd/air-traffic-server's depisolation test).

// PresidioDefaultScoreGate is the acceptance threshold applied to Presidio
// results unless an approved threshold rule lowers it for a specific type.
const PresidioDefaultScoreGate = 0.4

// PresidioEntityMap normalizes Presidio entity types onto the gateway's
// vocabulary. Types mapping to "" are dropped (DATE_TIME is FP noise against
// the semver/order-number traps). New engines/PII types must join this map
// — and add a validator guard in detect's typeGuards where checkable.
var PresidioEntityMap = map[string]string{
	"PERSON":        "PERSON_NAME",
	"LOCATION":      "ADDRESS",
	"EMAIL_ADDRESS": "EMAIL",
	"PHONE_NUMBER":  "PHONE",
	"US_SSN":        "SSN",
	"US_ITIN":       "SSN", // SSN-shaped tax id; inherits the SSN hyphen guard
	"CREDIT_CARD":   "CREDIT_CARD",
	"IP_ADDRESS":    "IP",
	"IBAN_CODE":     "IBAN",
	"MEDICAL_LICENSE": "MRN",
	"DATE_TIME":     "",
	"URL":           "",
	"NRP":           "",
}

// RuneToByteOffsets maps each rune index (plus one past the end) to its byte
// offset — Presidio speaks unicode codepoints, spine spans are bytes.
func RuneToByteOffsets(s string) []int {
	offs := make([]int, 0, len(s)+1)
	for i := range s {
		offs = append(offs, i)
	}
	offs = append(offs, len(s))
	return offs
}
