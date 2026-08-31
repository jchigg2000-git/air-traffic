package harness

// Behavioral recall is the harness's honesty check: it is measured against
// what the mock upstream actually received, not against what the detector
// claimed. That only holds while an unmeasurable request is excluded rather
// than assumed clean.

import (
	"testing"

	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
)

func ssnItem(content, value string) genItem {
	return genItem{
		Template: "t", Content: content,
		Truth: []model.TruthSpan{{Start: 0, End: len(value), Type: "SSN", Value: value}},
	}
}

func TestScoreRunUnjoinedCaptureIsNotPerfectRecall(t *testing.T) {
	st := store.New()
	st.AddGatewayReports([]model.GatewayRequestReport{{RequestID: "r1", Route: "anthropic", Action: "pass"}})

	// The report joined; the capture never did. Nothing is known about whether
	// the value reached the upstream.
	score, _ := scoreRun(st, "run1", []requestOutcome{
		{Item: ssnItem("123-45-6789 filed", "123-45-6789"), RequestID: "r1", HTTPStatus: 200},
	})

	if score.RecallBehavioral != 0 {
		t.Errorf("recall_behavioral = %.2f with no capture joined; want it held out, not reported perfect", score.RecallBehavioral)
	}
	if score.CaptureOrphans != 1 {
		t.Errorf("capture_orphans = %d, want 1", score.CaptureOrphans)
	}
}

// Blocking is enforcement, not an absent measurement: the request never
// reached the upstream, so there is no capture to join and the value counts
// as caught.
func TestScoreRunBlockedCountsCaught(t *testing.T) {
	st := store.New()
	st.AddGatewayReports([]model.GatewayRequestReport{{RequestID: "r1", Route: "anthropic", Action: "block"}})

	score, _ := scoreRun(st, "run1", []requestOutcome{
		{Item: ssnItem("123-45-6789 filed", "123-45-6789"), RequestID: "r1", HTTPStatus: 400},
	})

	if score.RecallBehavioral != 1 {
		t.Errorf("recall_behavioral = %.2f, want 1.00 for a blocked request", score.RecallBehavioral)
	}
	if score.CaptureOrphans != 0 {
		t.Errorf("capture_orphans = %d, want 0", score.CaptureOrphans)
	}
}

func TestScoreRunLeakedValueCountsAgainstRecall(t *testing.T) {
	st := store.New()
	st.AddGatewayReports([]model.GatewayRequestReport{{RequestID: "r1", Route: "anthropic", Action: "pass"}})
	st.RecordInferenceCapture(model.InferenceCapture{
		AdapterID: "anthropic", GatewayRequestID: "r1",
		Body: `{"messages":[{"role":"user","content":"123-45-6789 filed"}]}`,
	})

	score, _ := scoreRun(st, "run1", []requestOutcome{
		{Item: ssnItem("123-45-6789 filed", "123-45-6789"), RequestID: "r1", HTTPStatus: 200},
	})

	if score.RecallBehavioral != 0 {
		t.Errorf("recall_behavioral = %.2f, want 0 for a value that reached the upstream raw", score.RecallBehavioral)
	}
	if score.CaptureOrphans != 0 {
		t.Errorf("capture_orphans = %d, want 0 when the capture joined", score.CaptureOrphans)
	}
}
