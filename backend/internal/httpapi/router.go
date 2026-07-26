package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"market-analyzer/backend/internal/adapters/edgar"
	"market-analyzer/backend/internal/adapters/finnhub"
	"market-analyzer/backend/internal/adapters/telegram"
	"market-analyzer/backend/internal/config"
	"market-analyzer/backend/internal/prices"
)

// PriceProvider / AnnouncementProvider are satisfied both by the live adapter
// clients and by the DB-backed cache (internal/store), so the handler is
// unaware of whether a cache is present.
type PriceProvider interface {
	GetDailyPrices(ctx context.Context, ticker, from, to string) ([]prices.DailyPrice, error)
}

type AnnouncementProvider interface {
	GetAnnouncements(ctx context.Context, ticker string) ([]edgar.Announcement, error)
}

// Deps holds the long-lived clients shared across requests. They MUST be
// created once at startup and reused: finnhub.Client carries a mutex-guarded
// rate limiter and edgar.Client caches the CIK map, so a fresh client per
// request would defeat both.
type Deps struct {
	Finnhub  *finnhub.Client
	Telegram *telegram.Client
	Prices   PriceProvider
	Edgar    AnnouncementProvider
}

func NewRouter(cfg config.Config, deps Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("POST /telegram/webhook", handleTelegramWebhook(cfg, deps))

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
