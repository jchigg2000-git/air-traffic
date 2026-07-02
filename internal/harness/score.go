package harness

import (
	"encoding/json"
	"strings"

	"air-traffic/internal/model"
	"air-traffic/internal/store"
)

// requestOutcome is what the runner records per request before scoring.
type requestOutcome struct {
	Item         genItem
	RequestID    string
	HTTPStatus   int
	ResponseBody string
	IsSSE        bool
	LatencyMS    int64
	Err          string
}

// scoreRun joins each outcome against the gateway's per-request report (span
// precision/recall) and the mock upstream's capture (behavioral recall: did
// the raw value actually reach the "vendor"), and builds the per-request
// results the RedactionDiff view renders.
func scoreRun(st *store.Store, runID string, outcomes []requestOutcome) (model.RunScore, []model.HarnessResult) {
	score := model.RunScore{ByType: map[string]model.TypeScore{}, ByEngine: map[string]int{}}
	var results []model.HarnessResult
	var tp, fn, fp, behavioralCaught, behavioralTotal int

	for _, o := range outcomes {
		res := model.HarnessResult{
			RunID: runID, RequestID: o.RequestID, Template: o.Item.Template,
			Content: o.Item.Content, Truth: o.Item.Truth, Traps: o.Item.Traps,
			Straddle: o.Item.Straddle, HTTPStatus: o.HTTPStatus,
			LatencyMS: o.LatencyMS, Err: o.Err,
		}
		if o.Err != "" {
			results = append(results, res)
			continue
		}

		report, hasReport := st.GatewayReportByRequestID(o.RequestID)
		capture, hasCapture := st.InferenceCaptureByRequestID(o.RequestID)
		if hasReport {
			res.Action = report.Action
			res.Redactions = report.Redactions
			score.JoinedReports++
		} else {
			score.OrphanRequests++
			if o.HTTPStatus == 400 {
				res.Action = "block"
			}
		}
		if hasCapture {
			res.UpstreamText = upstreamContent(capture.Body)
		}

		// Behavioral recall: a seeded value present raw in the upstream
		// capture is a leak, whatever the detector claimed. Blocked requests
		// never reached upstream — everything counts as caught.
		for _, truth := range o.Item.Truth {
			behavioralTotal++
			leaked := hasCapture && strings.Contains(capture.Body, truth.Value)
			if leaked {
				ts := score.ByType[truth.Type]
				ts.BehavioralMisses++
				score.ByType[truth.Type] = ts
				res.MissedTypes = appendUnique(res.MissedTypes, truth.Type)
			} else {
				behavioralCaught++
			}
		}

		// Span-level precision/recall from the gateway's own report.
		matchedRedactions := map[int]bool{}
		for _, truth := range o.Item.Truth {
			matched := false
			for ri, red := range res.Redactions {
				if red.Type == truth.Type && red.Start < truth.End && truth.Start < red.End {
					matched, matchedRedactions[ri] = true, true
					break
				}
			}
			ts := score.ByType[truth.Type]
			if matched {
				tp++
				ts.TP++
			} else {
				fn++
				ts.FN++
				res.MissedTypes = appendUnique(res.MissedTypes, truth.Type)
			}
			score.ByType[truth.Type] = ts
		}
		for ri, red := range res.Redactions {
			score.ByEngine[red.Detector]++
			if matchedRedactions[ri] {
				continue
			}
			fp++
			ts := score.ByType[red.Type]
			ts.FP++
			score.ByType[red.Type] = ts
			for _, trap := range o.Item.Traps {
				if red.Start < trap.End && trap.Start < red.End {
					score.TrapFPs++
				}
			}
		}

		// Response-side scan (informational in this slice — enforcement on
		// responses is G4): reassemble SSE deltas, look for raw seeded values.
		respText := responseText(o.ResponseBody)
		if o.IsSSE {
			respText = reassembleSSE(o.ResponseBody)
		}
		if respText != "" {
			for _, truth := range o.Item.Truth {
				if strings.Contains(respText, truth.Value) {
					res.ResponseLeaks++
				}
			}
			score.ResponseLeaks += res.ResponseLeaks
		}

		results = append(results, res)
	}

	if tp+fn > 0 {
		score.RecallReported = float64(tp) / float64(tp+fn)
	}
	if tp+fp > 0 {
		score.Precision = float64(tp) / float64(tp+fp)
	}
	if behavioralTotal > 0 {
		score.RecallBehavioral = float64(behavioralCaught) / float64(behavioralTotal)
	}
	return score, results
}

// upstreamContent pulls messages[0].content out of the captured upstream body
// for the before/after diff view.
func upstreamContent(body string) string {
	var doc struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil || len(doc.Messages) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(doc.Messages[0].Content, &asString); err == nil {
		return asString
	}
	return ""
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
