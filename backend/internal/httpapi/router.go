package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"market-analyzer/backend/internal/config"
)

func NewRouter(cfg config.Config) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("GET /api/calendar", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "calendar endpoint scaffolded",
		})
	})

	mux.HandleFunc("GET /api/company/{ticker}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error":  "company endpoint scaffolded",
			"ticker": r.PathValue("ticker"),
		})
	})

	return withCORS(cfg.FrontendOrigin, mux)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
