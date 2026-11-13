// Package web serves the dashboard and the JSON API.
package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"

	"ipcheck/internal/agent"
)

//go:embed index.html
var indexHTML []byte

func Handler(a *agent.Agent) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.Snapshot())
	})

	mux.HandleFunc("POST /api/check", func(w http.ResponseWriter, r *http.Request) {
		a.CheckNow()
		w.WriteHeader(http.StatusAccepted)
	})

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")

		ch, cancel := a.Subscribe()
		defer cancel()
		for {
			data, _ := json.Marshal(a.Snapshot())
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-ch:
			}
		}
	})

	return withCORS(mux)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// withCORS lets other local clients (e.g. a Steam Deck widget) read the API.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h.ServeHTTP(w, r)
	})
}
