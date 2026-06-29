package emitter

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"air-traffic/internal/catalog"
	"air-traffic/internal/model"
	"air-traffic/internal/store"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

var batchRequiredKeys = []string{"contract", "batch_id", "connector", "collected_at", "window", "cursor", "complete", "observations", "errors"}

func validateBatch(t *testing.T, body map[string]any) {
	t.Helper()
	for _, k := range batchRequiredKeys {
		if _, ok := body[k]; !ok {
			t.Errorf("batch missing required key %q", k)
		}
	}
	if body["contract"] != model.ObservationContract {
		t.Errorf("wrong contract: %v", body["contract"])
	}
	conn, ok := body["connector"].(map[string]any)
	if !ok {
		t.Fatal("connector not an object")
	}
	for _, k := range []string{"type", "instance", "api_version"} {
		if _, ok := conn[k]; !ok {
			t.Errorf("connector missing %q", k)
		}
	}
}

func TestEmitTick_ProducesValidBatches(t *testing.T) {
	st := store.New()
	e := New(st, quietLogger(), time.Second)
	e.emitTick(time.Now().UTC())

	obs := st.ListObservations(1000)
	if len(obs) == 0 {
		t.Fatal("expected observations after a tick")
	}
	for _, o := range obs {
		validateBatch(t, o.Body)
		if o.ObservationCount != len(o.Body["observations"].([]any)) {
			t.Errorf("ObservationCount mismatch for %s", o.ConnectorInstance)
		}
	}
}

func TestEmitSynthetic_WalkWithinBounds(t *testing.T) {
	st := store.New()
	e := New(st, quietLogger(), time.Second)
	def, _ := catalog.ByID("openai")
	a, _ := st.GetAdapter("openai")
	for i := 0; i < 200; i++ {
		e.emitSynthetic(a, def.Metrics, time.Now().UTC())
	}
	for _, d := range def.Metrics {
		v := e.state[a.ID+"|"+d.Key]
		if v < d.Min-1e-9 || v > d.Max+1e-9 {
			t.Errorf("%s walked out of bounds: %v not in [%v,%v]", d.Key, v, d.Min, d.Max)
		}
	}
}

func TestDisabledAdapterNotEmitted(t *testing.T) {
	st := store.New()
	dis := model.ModeDisabled
	_, _ = st.PatchAdapter("openai", model.AdapterPatch{Mode: &dis})
	e := New(st, quietLogger(), time.Second)
	e.emitTick(time.Now().UTC())
	for _, o := range st.ListObservations(1000) {
		if o.ConnectorInstance == "openai" {
			t.Error("disabled adapter should not emit")
		}
	}
}

func TestRAG(t *testing.T) {
	if s, _ := rag(99.95, 99.9, 99.5, "higher"); s != "green" {
		t.Errorf("expected green, got %s", s)
	}
	if s, _ := rag(1.2, 0.5, 1.0, "lower"); s != "red" {
		t.Errorf("expected red, got %s", s)
	}
}
