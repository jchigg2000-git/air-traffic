package model

import "time"

// TruthSpan is one seeded value with its exact offsets into the generated
// content — ground truth by construction, never inferred.
type TruthSpan struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// HarnessRunConfig is the user-facing knobs for one run. Bounds are enforced
// server-side: Count ≤ 2000, Concurrency ≤ 8 (single-machine reality).
type HarnessRunConfig struct {
	Count               int   `json:"count"`
	Concurrency         int   `json:"concurrency"`
	Seed                int64 `json:"seed,omitempty"`
	IncludeTraps        bool  `json:"include_traps"`
	IncludePresidioOnly bool  `json:"include_presidio_only"`
	IncludeStraddle     bool  `json:"include_straddle"`
	ReplayPercent       int   `json:"replay_percent"`
}

// TypeScore is per-PII-type accounting.
type TypeScore struct {
	TP               int `json:"tp"`
	FN               int `json:"fn"`
	FP               int `json:"fp"`
	BehavioralMisses int `json:"behavioral_misses"`
}

// RunScore is the scored outcome of a completed run.
type RunScore struct {
	Precision        float64              `json:"precision"`
	RecallReported   float64              `json:"recall_reported"`
	RecallBehavioral float64              `json:"recall_behavioral"`
	TrapFPs          int                  `json:"trap_fps"`
	ResponseLeaks    int                  `json:"response_leaks"`
	ByType           map[string]TypeScore `json:"by_type"`
	ByEngine         map[string]int       `json:"by_engine"`
	JoinedReports    int                  `json:"joined_reports"`
	OrphanRequests   int                  `json:"orphan_requests"`
}

// HarnessRun is one traffic run through the gateway.
type HarnessRun struct {
	ID            string           `json:"id"`
	Config        HarnessRunConfig `json:"config"`
	Status        string           `json:"status"` // running | done | failed
	Total         int              `json:"total"`
	Completed     int              `json:"completed"`
	Masked        int              `json:"masked"`
	Blocked       int              `json:"blocked"`
	DetectOnly    int              `json:"detect_only"`
	Passed        int              `json:"passed"`
	Errors        int              `json:"errors"`
	PackVersion   int              `json:"pack_version"`
	DetectorChain string           `json:"detector_chain"`
	Score         *RunScore        `json:"score,omitempty"`
	PromotedCount int              `json:"promoted_count"`
	Error         string           `json:"error,omitempty"`
	StartedAt     time.Time        `json:"started_at"`
	FinishedAt    time.Time        `json:"finished_at,omitempty"`
}

// HarnessResult is one request's outcome, carrying everything the
// RedactionDiff view renders. Content and values are synthetic by
// construction — displaying them is safe, and that is the whole point.
type HarnessResult struct {
	RunID         string             `json:"run_id"`
	RequestID     string             `json:"request_id"`
	Template      string             `json:"template"`
	Content       string             `json:"content"`
	Truth         []TruthSpan        `json:"truth"`
	Traps         []TruthSpan        `json:"traps,omitempty"`
	Straddle      bool               `json:"straddle,omitempty"`
	Action        string             `json:"action"`
	HTTPStatus    int                `json:"http_status"`
	Redactions    []GatewayRedaction `json:"redactions,omitempty"`
	UpstreamText  string             `json:"upstream_text,omitempty"`
	MissedTypes   []string           `json:"missed_types,omitempty"`
	ResponseLeaks int                `json:"response_leaks,omitempty"`
	LatencyMS     int64              `json:"latency_ms"`
	Err           string             `json:"error,omitempty"`
}

// RatchetPoint is one point in the recall-ratchet series — the number this
// whole build exists to publish.
type RatchetPoint struct {
	RunID            string    `json:"run_id"`
	At               time.Time `json:"at"`
	CorpusVersion    string    `json:"corpus_version"`
	PackVersion      int       `json:"pack_version"`
	DetectorChain    string    `json:"detector_chain"`
	Precision        float64   `json:"precision"`
	RecallReported   float64   `json:"recall_reported"`
	RecallBehavioral float64   `json:"recall_behavioral"`
	RequestCount     int       `json:"request_count"`
	Seed             int64     `json:"seed"`
}

// CorpusEntry is one promoted miss: already synthetic, so no surrogate step
// is needed before durable storage (the harness IS the synthetic generator).
type CorpusEntry struct {
	ID        string      `json:"id"`
	Text      string      `json:"text"`
	Truth     []TruthSpan `json:"truth"`
	SourceRun string      `json:"source_run"`
	MissTypes []string    `json:"miss_types"`
	AddedAt   time.Time   `json:"added_at"`
}

// PatternProposal is one flywheel-suggested detector addition awaiting human
// review — v0 never auto-applies a pattern.
type PatternProposal struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Regex        string    `json:"regex"`
	Confidence   float64   `json:"confidence"`
	Rationale    string    `json:"rationale"`
	SampleMisses int       `json:"sample_misses"`
	Status       string    `json:"status"` // proposed | approved | rejected
	SourceRun    string    `json:"source_run"`
	CreatedAt    time.Time `json:"created_at"`
}
