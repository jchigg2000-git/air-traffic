package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jchigg2000-git/air-traffic/internal/store"
)

const maxBody = 2 << 20 // 2 MB

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// decodeJSON strictly decodes the request body into v (unknown fields rejected, 2 MB cap).
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func limitParam(r *http.Request, def int) int {
	q := r.URL.Query().Get("limit")
	if q == "" {
		return def
	}
	n := 0
	for _, c := range q {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 || n > 1000 {
		return def
	}
	return n
}
