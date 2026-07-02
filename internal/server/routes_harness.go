package server

import (
	"net/http"
	"strings"

	"air-traffic/internal/model"
)

// requireHarness guards all /api/harness/* routes: the engine is optional
// wiring (tests construct servers without it).
func (s *Server) requireHarness(w http.ResponseWriter) bool {
	if s.harness == nil {
		writeError(w, http.StatusServiceUnavailable, "harness engine not configured")
		return false
	}
	return true
}

func (s *Server) handleHarnessRuns(w http.ResponseWriter, r *http.Request) {
	if !s.requireHarness(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"runs": s.harness.Runs()})
	case http.MethodPost:
		var cfg model.HarnessRunConfig
		if err := decodeJSON(r, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid config: "+err.Error())
			return
		}
		run, err := s.harness.StartRun(cfg)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"run": run})
	default:
		writeError(w, http.StatusMethodNotAllowed, "use GET or POST")
	}
}

// handleHarnessRun serves /api/harness/runs/{id} and /{id}/results.
func (s *Server) handleHarnessRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireHarness(w) {
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, "/api/harness/runs/")
	parts := strings.Split(suffix, "/")
	id := parts[0]
	if id == "" {
		writeError(w, http.StatusNotFound, "missing run id")
		return
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	switch sub {
	case "":
		run, ok := s.harness.Run(id)
		if !ok {
			writeError(w, http.StatusNotFound, "unknown run")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"run": run})
	case "results":
		writeJSON(w, http.StatusOK, map[string]any{"results": s.harness.Results(id, limitParam(r, 500))})
	default:
		writeError(w, http.StatusNotFound, "unknown sub-resource")
	}
}

func (s *Server) handleHarnessRatchet(w http.ResponseWriter, r *http.Request) {
	if !s.requireHarness(w) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ratchet": s.harness.Ratchet()})
}

func (s *Server) handleHarnessCorpus(w http.ResponseWriter, r *http.Request) {
	if !s.requireHarness(w) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"corpus": s.harness.Corpus()})
}

// handleHarnessSample fires one ad-hoc prompt through the gateway and waits
// for the verdict — the UI's try-a-prompt box. The browser never talks to
// the gateway port directly; this is its stand-in.
func (s *Server) handleHarnessSample(w http.ResponseWriter, r *http.Request) {
	if !s.requireHarness(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		writeError(w, http.StatusBadRequest, "content is empty")
		return
	}
	sample, err := s.harness.RunSample(body.Content)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sample": sample})
}

// handleHarnessProposals serves the list plus /{id}/approve and /{id}/reject.
func (s *Server) handleHarnessProposals(w http.ResponseWriter, r *http.Request) {
	if !s.requireHarness(w) {
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, "/api/harness/proposals")
	suffix = strings.TrimPrefix(suffix, "/")
	if suffix == "" {
		writeJSON(w, http.StatusOK, map[string]any{"proposals": s.harness.Proposals()})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	parts := strings.Split(suffix, "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "use /api/harness/proposals/{id}/approve|reject")
		return
	}
	id, verb := parts[0], parts[1]
	switch verb {
	case "approve":
		pack, err := s.harness.ApproveProposal(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pack": pack, "proposals": s.harness.Proposals()})
	case "reject":
		if err := s.harness.RejectProposal(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"proposals": s.harness.Proposals()})
	default:
		writeError(w, http.StatusNotFound, "unknown verb "+verb)
	}
}
