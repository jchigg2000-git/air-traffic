package gateway

// The downward half of the spine contract: the control plane is the policy
// authority, the gateway is an enforcement point. Policy (baseline ⊕ vendor
// flags) derives the redaction action; the pattern pack hot-reloads detector
// additions. A Policy Editor change propagates within one pull interval —
// no redeploy (G7 acceptance).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/gateway/detect"
	"github.com/jchigg2000-git/air-traffic/internal/model"
)

// RunPolicyPull drives the pull loop until ctx is cancelled. Failures retry
// on a short fuse (boot race: the control plane may still be compiling).
func (s *Server) RunPolicyPull(ctx context.Context) {
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			delay := s.cfg.PolicyPullInterval
			if err := s.pullOnce(ctx); err != nil {
				delay = min(2*time.Second, s.cfg.PolicyPullInterval)
			}
			t.Reset(delay)
		}
	}
}

func (s *Server) pullOnce(ctx context.Context) error {
	var firstErr error
	if err := s.pullPolicy(ctx); err != nil {
		s.log.Warn("policy pull failed", "error", err)
		firstErr = err
	}
	if err := s.pullPatterns(ctx); err != nil {
		s.log.Warn("pattern pull failed", "error", err)
		if firstErr == nil {
			firstErr = err
		}
	}
	if err := s.pullKeys(ctx); err != nil {
		s.log.Warn("keystore pull failed", "error", err)
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// pullKeys refreshes the keystore snapshot, on the same version-compare-and-
// swap contract as the pattern pack.
//
// A failure here leaves the previous snapshot in place rather than clearing
// it. Failing closed would take every keystore client down whenever the
// control plane restarts — and its store is in-memory, so restarts are
// routine. The cost of that choice, stated plainly: revocation is eventual,
// bounded by GATEWAY_POLICY_PULL_INTERVAL (15s by default), and unbounded
// while the control plane is unreachable.
func (s *Server) pullKeys(ctx context.Context) error {
	var payload struct {
		Snapshot model.KeySnapshot `json:"snapshot"`
	}
	if err := s.getJSON(ctx, "/api/gateway/keys", &payload); err != nil {
		return err
	}
	if prev := s.keys.Load(); prev != nil && prev.version == payload.Snapshot.Version {
		return nil
	}
	s.keys.Store(newKeySnapshot(payload.Snapshot))
	s.log.Info("keystore reloaded",
		"version", payload.Snapshot.Version,
		"apps", len(payload.Snapshot.Apps),
		"keys", len(payload.Snapshot.Keys))
	return nil
}

func (s *Server) pullPolicy(ctx context.Context) error {
	if s.cfg.RedactAction != "per_policy" {
		return nil // action pinned by config; policy stays informational
	}
	var payload struct {
		Policy *struct {
			Baseline string                    `json:"baseline"`
			Vendors  map[string]map[string]any `json:"vendors"`
		} `json:"policy"`
	}
	if err := s.getJSON(ctx, "/api/policies", &payload); err != nil {
		return err
	}
	// Baselines are cached whole rather than resolved one at a time: an app
	// with its own baseline needs to derive an action per request, and that
	// cannot mean a fetch per request.
	byID, err := s.fetchBaselines(ctx)
	if err != nil {
		return err
	}
	s.baselines.Store(&byID)

	if payload.Policy == nil || payload.Policy.Baseline == "" {
		// No policy applied yet; keep whatever we are already enforcing rather
		// than reverting mid-flight.
		//
		// If we are ALREADY enforcing something, the two halves now disagree:
		// the control plane believes nothing is applied while this gateway is
		// still gating traffic under a previously-pulled action, and a restart
		// of THIS process would drop to the actionMask default — a third
		// posture matching neither belief. Since 2026-08-16 the control plane
		// persists its policy across restarts (internal/store/policy_persist.go),
		// so the common cause of this is gone; say so out loud when it happens
		// anyway rather than letting it stay silent. Turning the disagreement
		// into a first-class drift record is PIVOT-13 and is not done here.
		if prev, ok := s.policyAction.Load().(string); ok && prev != "" {
			s.log.Warn("control plane reports no applied policy while this gateway is still enforcing",
				"enforcing", prev, "baseline", s.policyBaseline.Load())
		}
		return nil
	}
	base, ok := byID[payload.Policy.Baseline]
	if !ok {
		return fmt.Errorf("baseline %q not found", payload.Policy.Baseline)
	}
	// The attestation is the operator's, made explicitly in the Rigor Console
	// before Apply commits (web/src/pages/RigorConsole.tsx). It rides the
	// policy as a vendor flag on the path model.ZDRAttestationVendor/Key, and
	// this is its only source — an unattested healthcare baseline is a
	// deliberate, previewed block, not an accident of an empty override map.
	zdrAttested := false
	if v, ok := payload.Policy.Vendors[model.ZDRAttestationVendor]; ok {
		zdrAttested, _ = v[model.ZDRAttestationKey].(bool)
	}
	// ZDR attestation is a property of the vendor contract, not of the
	// caller, so it stays global and feeds per-app derivation unchanged.
	s.zdrAttested.Store(zdrAttested)
	action := deriveAction(base, zdrAttested)
	if prev := s.policyAction.Load(); prev == nil || prev.(string) != action {
		s.log.Info("policy action changed", "baseline", payload.Policy.Baseline, "action", action, "zdr_attested", zdrAttested)
	}
	s.policyAction.Store(action)
	s.policyBaseline.Store(payload.Policy.Baseline)
	return nil
}

// deriveAction maps a baseline to the gateway action. The rule itself lives in
// model.GatewayAction, shared with the control plane so the Rigor Console's
// pre-commit preview and this enforcement path cannot be two implementations
// that drift apart.
func deriveAction(b model.Baseline, zdrAttested bool) string {
	return model.GatewayAction(b, zdrAttested)
}

func (s *Server) fetchBaselines(ctx context.Context) (map[string]model.Baseline, error) {
	var payload struct {
		Baselines []model.Baseline `json:"baselines"`
	}
	if err := s.getJSON(ctx, "/api/baselines", &payload); err != nil {
		return nil, err
	}
	byID := make(map[string]model.Baseline, len(payload.Baselines))
	for _, b := range payload.Baselines {
		byID[b.ID] = b
	}
	return byID, nil
}

func (s *Server) pullPatterns(ctx context.Context) error {
	var payload struct {
		Pack model.PatternPack `json:"pack"`
	}
	if err := s.getJSON(ctx, "/api/gateway/patterns", &payload); err != nil {
		return err
	}
	if int64(payload.Pack.Version) == s.packVersion.Load() {
		return nil
	}
	rules := make([]detect.PatternRule, 0, len(payload.Pack.Rules))
	for _, r := range payload.Pack.Rules {
		rules = append(rules, detect.PatternRule{
			ID: r.ID, Type: r.Type, Kind: r.Kind, Regex: r.Regex,
			DenyList: r.DenyList, AllowList: r.AllowList,
			Threshold: r.Threshold, Context: r.Context,
			Confidence: r.Confidence,
		})
	}
	// Regex first, because it is the only half that can refuse: it compiles
	// every rule into a new recognizer set before swapping it in, so a pack
	// with a bad regex is rejected having installed nothing. Installing the
	// chain's allow-list suppression first — the earlier ordering, argued from
	// "suppression should never lag an addition" — meant a rejected pack still
	// silenced terms while the recognizers it came with never loaded.
	//
	// The window that ordering was worried about survives, inverted: between
	// these two calls a pack that both adds a recognizer and suppresses a term
	// enforces the addition alone. That is microseconds of over-detection; the
	// old order bought it with indefinite under-detection.
	if s.regexDet != nil {
		if err := s.regexDet.SetPatternPack(rules); err != nil {
			// Record the version anyway: nothing was installed, and a pack the
			// control plane will keep serving unchanged must not re-fail on
			// every pull. The next version — the fixed one — reloads normally.
			s.packVersion.Store(int64(payload.Pack.Version))
			return fmt.Errorf("pattern pack v%d rejected: %w", payload.Pack.Version, err)
		}
	}
	s.chain.SetPatternPack(rules)
	if s.presidio != nil {
		s.presidio.SetPatternPack(rules)
	}
	s.packVersion.Store(int64(payload.Pack.Version))
	s.log.Info("pattern pack reloaded", "version", payload.Pack.Version, "rules", len(rules))
	return nil
}

func (s *Server) getJSON(ctx context.Context, path string, v any) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.ControlPlaneURL+path, nil)
	if err != nil {
		return err
	}
	s.authSpine(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %d", path, resp.StatusCode)
	}
	// Bound the control-plane read: every other read in this codebase is capped, but this one feeds
	// enforcement action on a periodic loop — a misconfigured/compromised control plane returning an
	// oversized or never-ending body must not let the gateway allocate unbounded memory.
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(v)
}
