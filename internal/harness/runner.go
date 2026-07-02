package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"air-traffic/internal/model"
	"air-traffic/internal/store"
)

const (
	maxRunCount       = 2000
	maxConcurrency    = 8
	maxStoredRuns     = 50
	maxStoredResults  = 2000 // per run; single active run at a time
	reportJoinTimeout = 25 * time.Second
)

// Runner owns harness run lifecycle, scores, and the flywheel state. One run
// at a time — this is a single-machine proof harness, not a load generator.
type Runner struct {
	store      *store.Store
	log        *slog.Logger
	persist    *persister
	gatewayKey string
	httpc      *http.Client

	mu        sync.Mutex
	runs      []*model.HarnessRun
	results   map[string][]model.HarnessResult
	ratchet   []model.RatchetPoint
	corpus    []model.CorpusEntry
	proposals []model.PatternProposal
	running   bool
}

// NewRunner loads persisted flywheel state and republishes the persisted
// pattern pack into the store so gateways pull it after a restart.
func NewRunner(st *store.Store, log *slog.Logger, dataDir, gatewayKey string) (*Runner, error) {
	p, err := newPersister(dataDir)
	if err != nil {
		return nil, err
	}
	pack, proposals := p.loadPatterns()
	st.SetPatternPack(pack)
	return &Runner{
		store: st, log: log, persist: p, gatewayKey: gatewayKey,
		httpc:   &http.Client{Timeout: 60 * time.Second},
		results: map[string][]model.HarnessResult{},
		ratchet: p.loadRatchet(),
		corpus:  p.loadCorpus(),
		proposals: proposals,
	}, nil
}

// StartRun validates config, resolves the gateway from its freshest
// heartbeat, and launches the run goroutine.
func (r *Runner) StartRun(cfg model.HarnessRunConfig) (*model.HarnessRun, error) {
	if cfg.Count <= 0 {
		cfg.Count = 200
	}
	if cfg.Count > maxRunCount {
		return nil, fmt.Errorf("count %d exceeds the single-machine bound %d", cfg.Count, maxRunCount)
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.Concurrency > maxConcurrency {
		cfg.Concurrency = maxConcurrency
	}
	if cfg.ReplayPercent < 0 || cfg.ReplayPercent > 90 {
		return nil, fmt.Errorf("replay_percent must be 0–90")
	}
	if cfg.Seed == 0 {
		cfg.Seed = time.Now().UnixNano()
	}

	gwURL, chain, err := r.freshGateway()
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil, fmt.Errorf("a run is already in progress")
	}
	run := &model.HarnessRun{
		ID: model.NewUUID(), Config: cfg, Status: "running", Total: cfg.Count,
		PackVersion: r.store.GetPatternPack().Version, DetectorChain: chain,
		StartedAt: time.Now().UTC(),
	}
	r.running = true
	r.runs = append(r.runs, run)
	if len(r.runs) > maxStoredRuns {
		r.runs = r.runs[len(r.runs)-maxStoredRuns:]
	}
	go r.execute(run, gwURL)
	return snapshotRun(run), nil
}

func (r *Runner) freshGateway() (string, string, error) {
	var freshest *model.EnforcementReport
	for _, rep := range r.store.ListGatewayEnforcement() {
		rep := rep
		if freshest == nil || rep.At.After(freshest.At) {
			freshest = &rep
		}
	}
	if freshest == nil || time.Since(freshest.At) > 45*time.Second {
		return "", "", fmt.Errorf("no fresh gateway heartbeat; start air-traffic-gateway first")
	}
	return freshest.BaseURL, strings.Join(freshest.Detectors, ","), nil
}

func (r *Runner) execute(run *model.HarnessRun, gwURL string) {
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	r.mu.Lock()
	corpusSnapshot := make([]model.CorpusEntry, len(r.corpus))
	copy(corpusSnapshot, r.corpus)
	r.mu.Unlock()

	items := generate(run.Config, corpusSnapshot)
	outcomes := make([]requestOutcome, len(items))

	var wg sync.WaitGroup
	sem := make(chan struct{}, run.Config.Concurrency)
	for i, item := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, item genItem) {
			defer wg.Done()
			defer func() { <-sem }()
			outcomes[i] = r.sendOne(gwURL, run.ID, i, item)
			r.mu.Lock()
			run.Completed++
			if outcomes[i].Err != "" {
				run.Errors++
			} else if outcomes[i].HTTPStatus == http.StatusBadRequest &&
				strings.Contains(outcomes[i].ResponseBody, "blocked by gateway policy") {
				run.Blocked++
			}
			r.mu.Unlock()
		}(i, item)
	}
	wg.Wait()

	r.waitForReports(outcomes)
	score, results := scoreRun(r.store, run.ID, outcomes)
	promoted := r.flywheel(run, results)
	point := model.RatchetPoint{
		RunID: run.ID, At: time.Now().UTC(),
		CorpusVersion: corpusVersion(corpusSnapshot), PackVersion: run.PackVersion,
		DetectorChain: run.DetectorChain,
		Precision:     score.Precision, RecallReported: score.RecallReported,
		RecallBehavioral: score.RecallBehavioral,
		RequestCount:     run.Total, Seed: run.Config.Seed,
	}
	if err := r.persist.appendRatchet(point); err != nil {
		r.log.Warn("ratchet persist failed", "error", err)
	}
	r.ingestRatchetObservation(point)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.ratchet = append(r.ratchet, point)
	if len(results) > maxStoredResults {
		results = results[:maxStoredResults]
	}
	r.results[run.ID] = results
	for _, res := range results {
		switch res.Action {
		case "mask":
			run.Masked++
		case "detect":
			run.DetectOnly++
		case "pass":
			run.Passed++
		}
	}
	run.Score = &score
	run.PromotedCount = promoted
	run.Status = "done"
	run.FinishedAt = time.Now().UTC()
	r.log.Info("harness run complete", "run", run.ID,
		"recall_behavioral", score.RecallBehavioral, "precision", score.Precision,
		"trap_fps", score.TrapFPs, "promoted", promoted)
}

func (r *Runner) sendOne(gwURL, runID string, i int, item genItem) requestOutcome {
	out := requestOutcome{Item: item, RequestID: fmt.Sprintf("hr-%s-%d", runID[:8], i), IsSSE: item.Straddle}
	body := map[string]any{
		"model":    "claude-harness",
		"stream":   item.Straddle,
		"messages": []map[string]any{{"role": "user", "content": item.Content}},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	req, err := http.NewRequest(http.MethodPost, gwURL+"/v1/messages", bytes.NewReader(raw))
	if err != nil {
		out.Err = err.Error()
		return out
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.gatewayKey)
	req.Header.Set("X-Gateway-Request-Id", out.RequestID)
	if item.Straddle {
		req.Header.Set("X-Harness-Echo", "input")
		req.Header.Set("X-Harness-Straddle", "on")
	}
	start := time.Now()
	resp, err := r.httpc.Do(req)
	out.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		out.Err = err.Error()
		return out
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		out.Err = err.Error()
		return out
	}
	out.HTTPStatus = resp.StatusCode
	out.ResponseBody = string(respBody)
	return out
}

// waitForReports polls until every successful request has its gateway report
// (the gateway pushes on its own cadence) or the join deadline passes —
// orphans are then reported honestly in the score.
func (r *Runner) waitForReports(outcomes []requestOutcome) {
	deadline := time.Now().Add(reportJoinTimeout)
	for time.Now().Before(deadline) {
		missing := 0
		for _, o := range outcomes {
			if o.Err != "" {
				continue
			}
			if _, ok := r.store.GatewayReportByRequestID(o.RequestID); !ok {
				missing++
			}
		}
		if missing == 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (r *Runner) ingestRatchetObservation(pt model.RatchetPoint) {
	obs := model.Obs("metric", "detector_recall_ratchet", pt.RecallBehavioral*100, "percent",
		"green", "info", model.PlaneObservability, "anthropic", model.DispProxyEnforced,
		map[string]any{"pack_version": pt.PackVersion, "detector_chain": pt.DetectorChain},
		"harness", "")
	body := map[string]any{
		"contract": model.ObservationContract,
		"batch_id": model.NewUUID(),
		"connector": map[string]any{
			"type": "harness", "instance": "harness", "api_version": "harness/v0",
		},
		"collected_at": pt.At.Format(time.RFC3339),
		"window": map[string]any{
			"from": pt.At.Format(time.RFC3339), "to": pt.At.Format(time.RFC3339),
		},
		"cursor":       map[string]any{"input": nil, "output": nil},
		"complete":     true,
		"observations": []any{obs},
		"errors":       []any{},
	}
	r.store.AddObservation(model.ObservationRecord{
		Contract: model.ObservationContract, ConnectorType: "harness",
		ConnectorInstance: "harness", Complete: true, ObservationCount: 1, Body: body,
	})
}

// ---- read accessors (route handlers) ----

func snapshotRun(run *model.HarnessRun) *model.HarnessRun {
	cp := *run
	return &cp
}

func (r *Runner) Runs() []*model.HarnessRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*model.HarnessRun, 0, len(r.runs))
	for i := len(r.runs) - 1; i >= 0; i-- {
		out = append(out, snapshotRun(r.runs[i]))
	}
	return out
}

func (r *Runner) Run(id string) (*model.HarnessRun, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.runs {
		if run.ID == id {
			return snapshotRun(run), true
		}
	}
	return nil, false
}

func (r *Runner) Results(id string, limit int) []model.HarnessResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	results := r.results[id]
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	out := make([]model.HarnessResult, len(results))
	copy(out, results)
	return out
}

func (r *Runner) Ratchet() []model.RatchetPoint {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.RatchetPoint, len(r.ratchet))
	copy(out, r.ratchet)
	return out
}

func (r *Runner) Corpus() []model.CorpusEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.CorpusEntry, len(r.corpus))
	copy(out, r.corpus)
	return out
}

func (r *Runner) Proposals() []model.PatternProposal {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.PatternProposal, len(r.proposals))
	copy(out, r.proposals)
	return out
}
