package gateway

import (
	"sort"
	"sync"
)

const latReservoirMax = 4096

// metrics aggregates per-window counters that the spine pusher drains into
// ops-observation-batch/v1 entries. Latency reservoirs are bounded per window
// so cardinality and memory stay flat under load (G8 does the real depth).
type metrics struct {
	mu               sync.Mutex
	requests         int64
	blocked          int64
	masked           int64
	detectOnly       int64
	failTrips        int64
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

type metricsSnapshot struct {
	Requests, Blocked, Masked, DetectOnly, FailTrips int64
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
		RedactionsByType: m.redactionsByType,
		TokensIn:         m.tokensIn, TokensOut: m.tokensOut,
		AddedP50: percentile(m.addedLat, 0.50), AddedP95: percentile(m.addedLat, 0.95), AddedP99: percentile(m.addedLat, 0.99),
		TotalP50: percentile(m.totalLat, 0.50), TotalP95: percentile(m.totalLat, 0.95), TotalP99: percentile(m.totalLat, 0.99),
	}
	m.requests, m.blocked, m.masked, m.detectOnly, m.failTrips = 0, 0, 0, 0, 0
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
