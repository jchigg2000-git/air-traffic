package model

import (
	"strings"
	"testing"
)

func TestGatewayRequestReportClamp(t *testing.T) {
	r := GatewayRequestReport{
		RequestID: strings.Repeat("r", 4096),
		Model:     strings.Repeat("m", 3<<20),
		Route:     strings.Repeat("x", 100),
		Subject:   strings.Repeat("s", 500),
	}
	for i := 0; i < 700; i++ {
		r.Redactions = append(r.Redactions, GatewayRedaction{Path: strings.Repeat("p", 400), Type: "SSN"})
	}
	for i := 0; i < 9; i++ {
		r.DetectorErrors = append(r.DetectorErrors, strings.Repeat("e", 1000))
	}
	r.Clamp()
	if len(r.RequestID) != maxReportIDLen || len(r.Model) != maxReportModelLen ||
		len(r.Route) != maxReportLabelLen || len(r.Subject) != maxReportSubjectLen {
		t.Errorf("string bounds not applied: id=%d model=%d route=%d subject=%d",
			len(r.RequestID), len(r.Model), len(r.Route), len(r.Subject))
	}
	if len(r.Redactions) != maxReportRedactions || len(r.Redactions[0].Path) != maxReportPathLen {
		t.Errorf("redactions = %d, path = %d", len(r.Redactions), len(r.Redactions[0].Path))
	}
	if len(r.DetectorErrors) != maxReportDetectorErrs || len(r.DetectorErrors[0]) != maxReportDetectorErr {
		t.Errorf("detector errors = %d, len = %d", len(r.DetectorErrors), len(r.DetectorErrors[0]))
	}
	short := GatewayRequestReport{RequestID: "hr-deadbeef-7", Model: "claude-x", Route: "anthropic"}
	short.Clamp()
	if short.RequestID != "hr-deadbeef-7" || short.Model != "claude-x" || short.Route != "anthropic" {
		t.Errorf("Clamp altered in-bounds fields: %+v", short)
	}
}
