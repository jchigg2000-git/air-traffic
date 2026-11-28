package server

// limitParam and statusForError back every list and store-backed route, and
// POST /api/harness/proposals is the owner's way into the propose → approve
// path for allow_list suppressions. None of these make network calls: the
// harness runner has no presidio URL and no run is started.

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jchigg2000-git/air-traffic/internal/harness"
	"github.com/jchigg2000-git/air-traffic/internal/store"
)

func TestLimitParam(t *testing.T) {
	const def = 100
	for _, c := range []struct {
		q    string
		want int
	}{
		{"", def}, {"50", 50}, {"1000", 1000}, {"1001", def}, {"0", def},
		{"-5", def}, {"12a", def}, {"1e3", def},
	} {
		path := "/x"
		if c.q != "" {
			path += "?limit=" + c.q
		}
		if got := limitParam(httptest.NewRequest(http.MethodGet, path, nil), def); got != c.want {
			t.Errorf("limit=%q: got %d, want %d", c.q, got, c.want)
		}
	}
}

func TestStatusForError(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want int
	}{
		{"not found", store.ErrNotFound, http.StatusNotFound},
		{"wrapped not found", fmt.Errorf("key k1: %w", store.ErrNotFound), http.StatusNotFound},
		{"invalid", store.ErrInvalid, http.StatusBadRequest},
		{"wrapped invalid", fmt.Errorf("adapter: %w", store.ErrInvalid), http.StatusBadRequest},
		{"unknown", errors.New("disk on fire"), http.StatusInternalServerError},
	} {
		if got := statusForError(c.err); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func harnessTestServer(t *testing.T) http.Handler {
	t.Helper()
	st := store.New()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner, err := harness.NewRunner(st, log, t.TempDir(), "gwk-test", "")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(st, log)
	srv.SetHarness(runner)
	return srv.Routes()
}

func TestHarnessOwnerProposalRoutes(t *testing.T) {
	h := harnessTestServer(t) // no admin key: the write gate is open

	code, body := req(t, h, "POST", "/api/harness/proposals", map[string]any{
		"type": "PERSON_NAME", "kind": "allow_list", "allow_list": []string{"Sherlock Holmes"},
		"rationale": "fictional character in sample prompts",
	})
	if code != http.StatusCreated {
		t.Fatalf("POST valid allow_list = %d %v, want 201", code, body)
	}
	created, _ := body["proposal"].(map[string]any)
	id, _ := created["id"].(string)
	if id == "" || created["status"] != "proposed" || created["source_run"] != "manual" {
		t.Fatalf("created = %v, want proposed manual proposal with an id", created)
	}
	code, body = req(t, h, "GET", "/api/harness/proposals", nil)
	listed := false
	if list, ok := body["proposals"].([]any); ok {
		for _, p := range list {
			if m, _ := p.(map[string]any); m["id"] == id {
				listed = true
			}
		}
	}
	if code != http.StatusOK || !listed {
		t.Errorf("GET proposals = %d %v, want %q listed", code, body, id)
	}

	for _, c := range []struct {
		name         string
		method, path string
		body         any
		want         int
	}{
		{"unknown field", "POST", "/api/harness/proposals", map[string]any{
			"type": "PERSON_NAME", "kind": "allow_list", "allow_list": []string{"Watson"},
			"rationale": "fictional", "auto_approve": true,
		}, http.StatusBadRequest},
		{"empty rationale", "POST", "/api/harness/proposals", map[string]any{
			"type": "PERSON_NAME", "kind": "allow_list", "allow_list": []string{"Watson"},
		}, http.StatusBadRequest},
		{"unknown run", "GET", "/api/harness/runs/nope", nil, http.StatusNotFound},
		{"missing run id", "GET", "/api/harness/runs/", nil, http.StatusNotFound},
		{"PUT proposals list", "PUT", "/api/harness/proposals", nil, http.StatusMethodNotAllowed},
		{"unknown verb", "POST", "/api/harness/proposals/x/frobnicate", nil, http.StatusNotFound},
	} {
		if code, body := req(t, h, c.method, c.path, c.body); code != c.want {
			t.Errorf("%s: %s %s = %d %v, want %d", c.name, c.method, c.path, code, body, c.want)
		}
	}

	// The rejected bodies never reached the queue.
	_, body = req(t, h, "GET", "/api/harness/proposals", nil)
	if list, _ := body["proposals"].([]any); len(list) != 1 {
		t.Errorf("proposals after rejected posts = %d, want 1", len(list))
	}
}
