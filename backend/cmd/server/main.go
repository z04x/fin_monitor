package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"market-analyzer/backend/internal/adapters/edgar"
	"market-analyzer/backend/internal/adapters/finnhub"
	"market-analyzer/backend/internal/adapters/telegram"
	"market-analyzer/backend/internal/config"
	"market-analyzer/backend/internal/db"
	"market-analyzer/backend/internal/httpapi"
	"market-analyzer/backend/internal/prices"
	"market-analyzer/backend/internal/store"
)

func main() {
	cfg := config.Load()

	pricesClient := prices.NewClient(cfg.TiingoBaseURL, cfg.TiingoToken)
	edgarClient := edgar.NewClient(cfg.EdgarUserAgent)

	deps := httpapi.Deps{
		Finnhub:  finnhub.NewClient(cfg.FinnhubBaseURL, cfg.FinnhubToken),
		Telegram: telegram.NewClient(cfg.TelegramBotToken),
		Prices:   pricesClient,
		Edgar:    edgarClient,
	}

	// Optional DB cache: with DATABASE_URL set, wrap the price/announcement
	// providers with the Postgres-backed store; without it, run live.
	if cfg.DatabaseURL != "" {
		pool, err := db.Open(cfg.DatabaseURL)
		if err != nil {
			slog.Error("db open failed — running without cache", "error", err)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if err := db.EnsureSchema(ctx, pool); err != nil {
				slog.Error("ensure schema failed — running without cache", "error", err)
			} else {
				st := store.New(pool, pricesClient, edgarClient)
				deps.Prices = st
				deps.Edgar = st
				slog.Info("price/announcement cache enabled")
			}
			cancel()
		}
	}

	router := httpapi.NewRouter(cfg, deps)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("starting api server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
