package audit

import (
	"reflect"
	"testing"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

func TestToSIEMFieldMapping(t *testing.T) {
	// A non-UTC timestamp must come out normalized to UTC with millisecond precision.
	ts := time.Date(2026, 3, 4, 10, 20, 30, 123456789, time.FixedZone("EST", -5*3600))
	before := map[string]any{"mode": "open"}
	after := map[string]any{"mode": "allow_list"}
	in := []model.AuditEvent{{
		ID:             "evt-1",
		Timestamp:      ts,
		Actor:          "ada-admin",
		Action:         "model_access.set",
		Resource:       "OpenAI",
		Plane:          model.PlaneDeveloperWorkflow,
		Vendor:         "openai",
		ControlSurface: model.DispVendorNative,
		Before:         before,
		After:          after,
		RequestID:      "req-1",
	}}

	got := ToSIEM(in)
	if len(got) != 1 {
		t.Fatalf("len(ToSIEM) = %d, want 1", len(got))
	}
	want := map[string]any{
		"@timestamp":          "2026-03-04T15:20:30.123Z",
		"event.id":            "evt-1",
		"event.action":        "model_access.set",
		"event.category":      "developer_workflow",
		"user.name":           "ada-admin",
		"cloud.provider":      "openai",
		"gen_ai.system":       "openai",
		"airtraffic.surface":  "vendor_native",
		"airtraffic.resource": "OpenAI",
		"airtraffic.before":   before,
		"airtraffic.after":    after,
		"trace.id":            "req-1",
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("ToSIEM record mismatch\n got: %#v\nwant: %#v", got[0], want)
	}
}

func TestToSIEMEmptyIsNonNil(t *testing.T) {
	got := ToSIEM(nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("ToSIEM(nil) = %#v, want empty non-nil slice", got)
	}
}
