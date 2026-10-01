package gateway

import (
	"sort"
	"sync"
)

const latReservoirMax = 4096

// routeMetrics keeps one metrics window per route, so an aggregate is
// attributed to the route that served it rather than to whichever vendor the
// emitter was written for. The route label is the one the per-request feed
// already carries (RequestAudit.Route): a wire dialect, not a vendor lookup,
// because a gateway cannot know who is really behind an OpenAI-compatible
// upstream. Routes are the fixed dialect set, so cardinality is bounded.
type routeMetrics struct {
	mu      sync.Mutex
	byRoute map[string]*metrics
}

func newRouteMetrics() *routeMetrics { return &routeMetrics{byRoute: map[string]*metrics{}} }

func (r *routeMetrics) window(route string) *metrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.byRoute[route]
	if !ok {
		m = newMetrics()
		r.byRoute[route] = m
	}
	return m
}

func (r *routeMetrics) observe(a RequestAudit, tokensIn, tokensOut int64) {
	r.window(a.Route).observe(a, tokensIn, tokensOut)
}

func (r *routeMetrics) authFailure(route string) { r.window(route).authFailure() }

// drain snapshots and resets every route's window, keeping only the routes
// that saw something, so an idle route emits nothing.
func (r *routeMetrics) drain() map[string]metricsSnapshot {
	r.mu.Lock()
	windows := make(map[string]*metrics, len(r.byRoute))
	for route, m := range r.byRoute {
		windows[route] = m
	}
	r.mu.Unlock()
	out := map[string]metricsSnapshot{}
	for route, m := range windows {
		if snap := m.drain(); snap.Requests > 0 || snap.AuthFailures > 0 {
			out[route] = snap
		}
	}
	return out
}

// metrics aggregates one route's per-window counters that the spine pusher
// drains into ops-observation-batch/v1 entries. Latency reservoirs are
// bounded per window so cardinality and memory stay flat under load (G8 does
// the real depth).
type metrics struct {
	mu               sync.Mutex
	requests         int64
	blocked          int64
	masked           int64
	detectOnly       int64
	failTrips        int64
	errors           int64
	authFailures     int64
	redactionsByType map[string]int64
	tokensIn         int64
	tokensOut        int64
	addedLat         []float64
	totalLat         []float64
}

func newMetrics() *metrics { return &metrics{redactionsByType: map[string]int64{}} }

func (m *metrics) observe(a RequestAudit, tokensIn, tokensOut int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests++
	switch a.Action {
	case actionBlock:
		m.blocked++
	case actionMask:
		m.masked++
	case actionDetect:
		m.detectOnly++
	}
	if a.FailModeTripped {
		m.failTrips++
	}
	if a.Error != "" {
		m.errors++
	}
	for _, r := range a.Redactions {
		m.redactionsByType[r.Type]++
	}
	m.tokensIn += tokensIn
	m.tokensOut += tokensOut
	if len(m.addedLat) < latReservoirMax {
		m.addedLat = append(m.addedLat, float64(a.AddedLatencyMS))
	}
	if len(m.totalLat) < latReservoirMax {
		m.totalLat = append(m.totalLat, float64(a.LatencyMS))
	}
}

// authFailure counts a request refused at authentication. It is deliberately
// outside requests: the caller never reached the pipeline, so folding it into
// the block-rate and error-rate denominators would let unauthenticated noise
// dilute the rates that describe real traffic.
func (m *metrics) authFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authFailures++
}

type metricsSnapshot struct {
	Requests, Blocked, Masked, DetectOnly, FailTrips int64
	Errors, AuthFailures                             int64
	RedactionsByType                                 map[string]int64
	TokensIn, TokensOut                              int64
	AddedP50, AddedP95, AddedP99                     float64
	TotalP50, TotalP95, TotalP99                     float64
}

// drain snapshots and resets the window.
func (m *metrics) drain() metricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := metricsSnapshot{
		Requests: m.requests, Blocked: m.blocked, Masked: m.masked,
		DetectOnly: m.detectOnly, FailTrips: m.failTrips,
		Errors: m.errors, AuthFailures: m.authFailures,
		RedactionsByType: m.redactionsByType,
		TokensIn:         m.tokensIn, TokensOut: m.tokensOut,
		AddedP50: percentile(m.addedLat, 0.50), AddedP95: percentile(m.addedLat, 0.95), AddedP99: percentile(m.addedLat, 0.99),
		TotalP50: percentile(m.totalLat, 0.50), TotalP95: percentile(m.totalLat, 0.95), TotalP99: percentile(m.totalLat, 0.99),
	}
	m.requests, m.blocked, m.masked, m.detectOnly, m.failTrips = 0, 0, 0, 0, 0
	m.errors, m.authFailures = 0, 0
	m.tokensIn, m.tokensOut = 0, 0
	m.redactionsByType = map[string]int64{}
	m.addedLat, m.totalLat = nil, nil
	return snap
}

func percentile(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := make([]float64, len(xs))
	copy(sorted, xs)
	sort.Float64s(sorted)
	idx := int(q * float64(len(sorted)-1))
	return sorted[idx]
}
