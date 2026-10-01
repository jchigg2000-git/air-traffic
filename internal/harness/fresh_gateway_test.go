package harness

import (
	"strings"
	"testing"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
)

// The harness trusts a heartbeat for exactly the window the control plane
// reports as "fresh" (model.GatewayStaleAfter). Before the constant was
// shared, this check carried its own literal and could disagree with the
// status page about whether a gateway was alive.
func TestFreshGatewayUsesSharedStaleWindow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		age     time.Duration
		wantErr bool
	}{
		{"just inside the window", model.GatewayStaleAfter - 5*time.Second, false},
		{"just outside the window", model.GatewayStaleAfter + 5*time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := store.New()
			st.SetGatewayEnforcement(model.EnforcementReport{
				GatewayID: "gw@test",
				BaseURL:   "http://127.0.0.1:8125",
				Detectors: []string{"regex"},
				At:        time.Now().UTC().Add(-tc.age),
			})
			r := &Runner{store: st}
			url, chain, err := r.freshGateway()
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "no fresh gateway heartbeat") {
					t.Fatalf("age %v: err = %v, want a no-fresh-heartbeat refusal", tc.age, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("age %v: unexpected refusal: %v", tc.age, err)
			}
			if url != "http://127.0.0.1:8125" || chain != "regex" {
				t.Errorf("freshGateway = (%q, %q)", url, chain)
			}
		})
	}
}

// One missed beat is jitter, three is an outage: the stale window is a
// multiple of the heartbeat interval, not an independent number.
func TestStaleWindowIsThreeHeartbeats(t *testing.T) {
	if model.GatewayStaleAfter != 3*model.GatewayHeartbeatInterval {
		t.Fatalf("stale window %v is not 3x the heartbeat interval %v",
			model.GatewayStaleAfter, model.GatewayHeartbeatInterval)
	}
}
