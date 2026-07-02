package harness_test

// Full-loop integration: control plane + mock upstream (same process),
// real gateway binary wiring (Server via httptest), harness runner, flywheel
// promotion, human approval, hot-reload, and a second run whose recall
// strictly improves — the entire Phase 3 story in one test.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"air-traffic/internal/gateway"
	"air-traffic/internal/gateway/config"
	"air-traffic/internal/harness"
	"air-traffic/internal/model"
	"air-traffic/internal/server"
	"air-traffic/internal/store"
)

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func waitDone(t *testing.T, hr *harness.Runner, id string) *model.HarnessRun {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if run, ok := hr.Run(id); ok && run.Status != "running" {
			return run
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("run did not finish in time")
	return nil
}

func TestFullLoopFlywheelRatchet(t *testing.T) {
	st := store.New()
	log := discard()

	cp := server.New(st, log)
	hr, err := harness.NewRunner(st, log, t.TempDir(), "gwk-test", "")
	if err != nil {
		t.Fatal(err)
	}
	cp.SetHarness(hr)
	cpSrv := httptest.NewServer(cp.Routes())
	defer cpSrv.Close()

	// Bind the gateway's listener FIRST so its advertised URL (default:
	// http://<listen addr>) is truthful and the runner discovers it via the
	// real heartbeat path. The old seeded-override approach left the real
	// heartbeat advertising the unbound default 127.0.0.1:8125 — freshest
	// wins in freshGateway, so later traffic silently targeted whatever dev
	// stack happened to be on that port and 401s scored as perfect recall.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_LISTEN_ADDR", ln.Addr().String())
	t.Setenv("GATEWAY_UPSTREAMS", `{"anthropic":{"base_url":"`+cpSrv.URL+`/synthetic/anthropic","credential_ref":"env:UP_CRED"}}`)
	t.Setenv("UP_CRED", "sk-ant-synthetic-test")
	t.Setenv("GATEWAY_CLIENT_KEYS_REF", "env:GW_KEYS")
	t.Setenv("GW_KEYS", "gwk-test")
	t.Setenv("GATEWAY_CONTROL_PLANE_URL", cpSrv.URL)
	t.Setenv("GATEWAY_REDACT_ACTION", "mask")
	t.Setenv("GATEWAY_OBS_PUSH_INTERVAL", "200ms")
	t.Setenv("GATEWAY_POLICY_PULL_INTERVAL", "200ms")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	gw, err := gateway.New(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	gwSrv := httptest.NewUnstartedServer(gw.Routes())
	gwSrv.Listener.Close()
	gwSrv.Listener = ln
	gwSrv.Start()
	defer gwSrv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go gw.RunSpine(ctx)
	go gw.RunPolicyPull(ctx)

	// The first heartbeat fires immediately but asynchronously — wait for
	// discovery instead of seeding an override.
	runCfg := model.HarnessRunConfig{Count: 60, Concurrency: 4, Seed: 42, IncludeTraps: true, IncludeStraddle: true}
	var run1 *model.HarnessRun
	discovery := time.Now().Add(10 * time.Second)
	for {
		run1, err = hr.StartRun(runCfg)
		if err == nil {
			break
		}
		if time.Now().After(discovery) {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	done1 := waitDone(t, hr, run1.ID)
	if done1.Status != "done" {
		t.Fatalf("run1 = %+v", done1)
	}
	s1 := done1.Score
	if s1 == nil {
		t.Fatal("run1 has no score")
	}
	if s1.OrphanRequests != 0 {
		t.Errorf("orphan requests = %d, want full join coverage", s1.OrphanRequests)
	}
	if s1.TrapFPs != 0 {
		t.Errorf("trap FPs = %d, want 0", s1.TrapFPs)
	}
	if s1.RecallBehavioral >= 1.0 {
		t.Errorf("recall_behavioral = %.3f; bare-SSN miss fodder should exist (flywheel premise)", s1.RecallBehavioral)
	}
	if s1.RecallBehavioral < 0.5 {
		t.Errorf("recall_behavioral = %.3f suspiciously low", s1.RecallBehavioral)
	}
	if s1.Precision < 0.9 {
		t.Errorf("precision = %.3f", s1.Precision)
	}
	if done1.PromotedCount == 0 {
		t.Error("no misses promoted to corpus")
	}

	// The curated candidate for bare SSNs must be proposed, not auto-applied.
	var proposal *model.PatternProposal
	for _, p := range hr.Proposals() {
		if p.ID == "ssn-bare-context" {
			p := p
			proposal = &p
		}
	}
	if proposal == nil || proposal.Status != "proposed" {
		t.Fatalf("ssn-bare-context proposal missing or wrong status: %+v", proposal)
	}

	// Approve → pack v1 → gateway hot-reloads via its pull loop.
	pack, err := hr.ApproveProposal("ssn-bare-context")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != 1 {
		t.Errorf("pack version = %d", pack.Version)
	}
	time.Sleep(600 * time.Millisecond) // ≥ one pull interval

	run2, err := hr.StartRun(runCfg) // same seed: identical traffic
	if err != nil {
		t.Fatal(err)
	}
	done2 := waitDone(t, hr, run2.ID)
	s2 := done2.Score
	if s2 == nil {
		t.Fatal("run2 has no score")
	}
	if s2.RecallBehavioral <= s1.RecallBehavioral {
		t.Errorf("ratchet did not move: run1 recall %.3f, run2 recall %.3f (pack v%d)",
			s1.RecallBehavioral, s2.RecallBehavioral, done2.PackVersion)
	}

	points := hr.Ratchet()
	if len(points) != 2 {
		t.Fatalf("ratchet points = %d", len(points))
	}
	if points[1].PackVersion != 1 || points[0].PackVersion != 0 {
		t.Errorf("ratchet pack versions = %d, %d", points[0].PackVersion, points[1].PackVersion)
	}

	// Leak guard at the control plane: no seeded value in audit or reports.
	values := []string{}
	for _, res := range hr.Results(run1.ID, 50) {
		for _, truth := range res.Truth {
			values = append(values, truth.Value)
		}
	}
	auditJSON, _ := json.Marshal(st.ListAudit(2000))
	reportsJSON, _ := json.Marshal(st.ListGatewayReports(2000))
	for _, v := range values {
		if strings.Contains(string(auditJSON), v) {
			t.Errorf("audit stream leaked seeded value %q", v)
		}
		if strings.Contains(string(reportsJSON), v) {
			t.Errorf("gateway reports leaked seeded value %q", v)
		}
	}

	// Try-a-prompt path: one ad-hoc sample through the same stack. It must
	// join the gateway report, mask the SSN before upstream, carry a model
	// reply — and leave the flywheel untouched (no run, no promotion).
	corpusBefore := len(hr.Corpus())
	sample, err := hr.RunSample("Reply to Jane, SSN 123-45-6789, at jane@example.org today.")
	if err != nil {
		t.Fatalf("RunSample: %v", err)
	}
	if sample.Action != "mask" || !sample.ReportJoined {
		t.Errorf("sample = %+v, want joined mask verdict", sample)
	}
	if len(sample.Redactions) == 0 {
		t.Error("sample carries no redactions")
	}
	if sample.UpstreamText == "" || strings.Contains(sample.UpstreamText, "123-45-6789") {
		t.Errorf("upstream text = %q, want masked SSN", sample.UpstreamText)
	}
	if sample.ResponseText == "" {
		t.Error("sample carries no model reply")
	}
	if len(hr.Corpus()) != corpusBefore {
		t.Error("an ad-hoc sample must never promote to the corpus")
	}
}
