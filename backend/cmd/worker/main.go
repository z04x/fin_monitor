package main

import (
	"log/slog"

	"market-analyzer/backend/internal/config"
)

func main() {
	cfg := config.Load()
	slog.Info("worker scaffold ready", "calendar_window_days", cfg.CalendarWindowDays)
}
