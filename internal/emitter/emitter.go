// Package emitter is the background observation generator. Each tick it produces
// one ops-observation-batch/v1 batch per emitting adapter (mirrors it-scorecard).
package emitter

import (
	"context"
	"log/slog"
	"math/rand"
	"strconv"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/catalog"
	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
)

// Emitter walks catalog metric defs and emits synthetic batches.
type Emitter struct {
	store        *store.Store
	log          *slog.Logger
	interval     time.Duration
	historyDepth int
	state        map[string]float64
	hooks        []func(ts time.Time)
}

// New builds an emitter. historyDepth backfill ticks are produced by Seed().
func New(st *store.Store, log *slog.Logger, interval time.Duration) *Emitter {
	return &Emitter{store: st, log: log, interval: interval, historyDepth: 12, state: map[string]float64{}}
}

// AddTickHook registers a function run after each tick's metric emission
// (used to refresh drift, etc.).
func (e *Emitter) AddTickHook(f func(ts time.Time)) { e.hooks = append(e.hooks, f) }

// Seed backfills historyDepth ticks before live emission begins.
func (e *Emitter) Seed() {
	now := time.Now().UTC()
	for k := e.historyDepth - 1; k >= 0; k-- {
		e.emitTick(now.Add(-time.Duration(k) * e.interval))
	}
	e.store.RecordActivity("emitter", "Seeded synthetic observation history", strconv.Itoa(e.historyDepth)+" ticks")
}

// Run drives live emission until ctx is cancelled.
func (e *Emitter) Run(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	e.store.RecordActivity("emitter", "Started synthetic emitter", e.interval.String())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.emitTick(time.Now().UTC())
		}
	}
}

func (e *Emitter) emitTick(ts time.Time) {
	for _, a := range e.store.ListAdapters() {
		if !a.Enabled || !a.Emit || a.Mode == model.ModeDisabled {
			continue
		}
		if a.Mode == model.ModeProxy {
			e.emitProxyStub(a, ts)
			continue
		}
		def, ok := catalog.ByID(a.ID)
		if !ok || len(def.Metrics) == 0 {
			continue
		}
		e.emitSynthetic(a, def, ts)
	}
	for _, h := range e.hooks {
		h(ts)
	}
}

func (e *Emitter) emitSynthetic(a model.Adapter, def catalog.Definition, ts time.Time) {
	defs := def.Metrics
	observations := make([]any, 0, len(defs)+8)
	agg := make(map[string]float64, len(defs))
	for _, d := range defs {
		key := a.ID + "|" + d.Key
		cur, seen := e.state[key]
		if !seen {
			cur = d.Baseline
		}
		var next float64
		if d.Step == 0 { // state metric — hold steady
			next = d.Baseline
		} else {
			delta := (rand.Float64()*2 - 1) * d.Step
			next = cur + delta + (d.Baseline-cur)*0.06
			if next < d.Min {
				next = d.Min
			}
			if next > d.Max {
				next = d.Max
			}
		}
		e.state[key] = next
		agg[d.Key] = next
		value := roundTo(next, d.Decimals)
		status, severity := rag(next, d.Green, d.Amber, d.Polarity)
		dims := map[string]any{}
		if d.Model != "" {
			dims["model"] = d.Model
		}
		dims["team"] = "platform-engineering"
		observations = append(observations, model.Obs(d.Kind, d.Key, value, d.Unit, status, severity, d.Plane, a.ID, d.Surface, dims, string(d.Surface), d.SourceURL))
	}
	observations = append(observations, e.costBreakdowns(a, def, agg)...)
	e.storeBatch(a, ts, observations, nil, true)
}

// costBreakdowns turns the vendor's current aggregate spend/usage into one
// "cost_breakdown" observation per supported facet member (cost by user / model / repo /
// project / …), so the drill-down rides the same ops-observation-batch/v1 contract every
// screen already consumes. Per-member share is normalized so the breakdown always sums to
// the vendor's aggregate cost_usd; per-member tokens & request counts ride as dimensions.
// Reads the same catalog.CostFacets the byte-identical synthetic surface serves.
func (e *Emitter) costBreakdowns(a model.Adapter, def catalog.Definition, agg map[string]float64) []any {
	cost := agg["cost_usd"]
	if cost <= 0 || len(def.CostFacets) == 0 {
		return nil
	}
	tin, tout := agg["tokens_in"], agg["tokens_out"]
	var out []any
	for _, f := range def.CostFacets {
		if !f.Supported || len(f.Members) == 0 {
			continue
		}
		var total float64
		for _, m := range f.Members {
			total += m.Weight
		}
		if total <= 0 {
			continue
		}
		for _, m := range f.Members {
			share := m.Weight / total
			dims := map[string]any{
				"cost_facet":        f.Dimension,
				"cost_facet_member": m.Key,
				"cost_facet_label":  m.Label,
				"tokens_in":         int64(tin*share + 0.5),
				"tokens_out":        int64(tout*share + 0.5),
				"requests":          int64(tin*share/150 + 0.5),
			}
			out = append(out, model.Obs("cost_breakdown", "cost_usd", roundTo(cost*share, 2), "usd", "green", "info",
				model.PlaneBudget, a.ID, model.DispVendorNative, dims, "cost-facets", f.Endpoint))
		}
	}
	return out
}

func (e *Emitter) emitProxyStub(a model.Adapter, ts time.Time) {
	errEntry := map[string]any{
		"scope": "connector", "code": "proxy_not_normalized",
		"message": "proxy mode reached upstream but no vendor normalizer is implemented in Phase 1", "retryable": false, "http_status": nil,
	}
	e.storeBatch(a, ts, nil, []any{errEntry}, false)
}

func (e *Emitter) storeBatch(a model.Adapter, ts time.Time, observations, errs []any, complete bool) {
	body := model.BuildBatch(a, ts, e.interval, observations, errs)
	e.store.AddObservation(model.ObservationRecord{
		ReceivedAt:        ts,
		Contract:          model.ObservationContract,
		ConnectorType:     "ai-vendor",
		ConnectorInstance: a.ID,
		Complete:          complete,
		ObservationCount:  len(observations),
		ErrorCount:        len(errs),
		Body:              body,
	})
}

func rag(value, green, amber float64, polarity string) (string, string) {
	if polarity == "higher" {
		switch {
		case value >= green:
			return "green", "info"
		case value >= amber:
			return "amber", "warning"
		default:
			return "red", "error"
		}
	}
	switch {
	case value <= green:
		return "green", "info"
	case value <= amber:
		return "amber", "warning"
	default:
		return "red", "error"
	}
}

func roundTo(v float64, decimals int) any {
	if decimals <= 0 {
		return int64(v + 0.5)
	}
	p := 1.0
	for i := 0; i < decimals; i++ {
		p *= 10
	}
	return float64(int64(v*p+0.5)) / p
}
