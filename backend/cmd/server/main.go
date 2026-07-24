package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"market-analyzer/backend/internal/config"
	"market-analyzer/backend/internal/httpapi"
)

func main() {
	cfg := config.Load()
	router := httpapi.NewRouter(cfg)

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
