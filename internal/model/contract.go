package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewUUID returns a random RFC-4122-ish v4 UUID string (stdlib only).
func NewUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]))
}

// BuildBatch assembles a schema-valid ops-observation-batch/v1 body.
func BuildBatch(adapter Adapter, ts time.Time, window time.Duration, observations, errors []any) map[string]any {
	if observations == nil {
		observations = []any{}
	}
	if errors == nil {
		errors = []any{}
	}
	return map[string]any{
		"contract": ObservationContract,
		"batch_id": NewUUID(),
		"connector": map[string]any{
			"type":        "ai-vendor",
			"instance":    adapter.ID,
			"api_version": adapter.APIVersion,
		},
		"collected_at": ts.UTC().Format(time.RFC3339),
		"window": map[string]any{
			"from": ts.Add(-window).UTC().Format(time.RFC3339),
			"to":   ts.UTC().Format(time.RFC3339),
		},
		"cursor":       map[string]any{"input": nil, "output": nil},
		"complete":     len(errors) == 0,
		"observations": observations,
		"errors":       errors,
	}
}

// Obs builds one observation entry with the standard dimension set.
func Obs(kind, name string, value any, unit, status, severity string, plane Plane, vendor string, surface Disposition, extraDims map[string]any, fixture, sourceURL string) any {
	dims := map[string]any{
		"plane":           string(plane),
		"vendor":          vendor,
		"control_surface": string(surface),
	}
	for k, v := range extraDims {
		dims[k] = v
	}
	return map[string]any{
		"kind": kind,
		"signal": map[string]any{
			"name":     name,
			"value":    value,
			"unit":     unit,
			"status":   status,
			"severity": severity,
		},
		"dimensions": dims,
		"provenance": map[string]any{
			"fixture":    fixture,
			"source_url": sourceURL,
		},
	}
}
