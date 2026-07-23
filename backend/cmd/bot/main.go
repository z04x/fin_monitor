package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"market-analyzer/backend/internal/adapters/finnhub"
	"market-analyzer/backend/internal/adapters/telegram"
	"market-analyzer/backend/internal/config"
	"market-analyzer/backend/internal/domain/digest"
)

func main() {
	if err := run(); err != nil {
		slog.Error("digest run failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		return err
	}
	today := time.Now().In(loc)
	if override := os.Getenv("DIGEST_DATE"); override != "" {
		today, err = time.ParseInLocation("2006-01-02", override, loc)
		if err != nil {
			return err
		}
	}
	dateStr := today.Format("2006-01-02")

	fh := finnhub.NewClient(cfg.FinnhubBaseURL, cfg.FinnhubToken)

	rawEvents, err := fh.GetCalendar(ctx, dateStr, dateStr)
	if err != nil {
		return err
	}
	slog.Info("fetched calendar", "date", dateStr, "raw_count", len(rawEvents))

	events := make([]digest.Event, 0, len(rawEvents))
	for _, raw := range rawEvents {
		e := digest.Event{
			Ticker:          raw.Symbol,
			Hour:            raw.Hour,
			EPSEstimate:     raw.EPSEstimate,
			EPSActual:       raw.EPSActual,
			RevenueEstimate: raw.RevenueEstimate,
		}
		if digest.HasEstimates(e) {
			if profile, err := fh.GetProfile(ctx, raw.Symbol); err == nil {
				e.CompanyName = profile.Name
			} else {
				slog.Warn("profile lookup failed", "ticker", raw.Symbol, "error", err)
			}
		}
		events = append(events, e)
	}

	text := digest.Build(today, events)

	tg := telegram.NewClient(cfg.TelegramBotToken)
	for _, chunk := range digest.Chunk(text, digest.TelegramMessageLimit) {
		if err := tg.SendMessage(ctx, cfg.TelegramChatID, chunk); err != nil {
			return err
		}
	}

	slog.Info("digest sent", "date", dateStr, "events_after_filter", len(digest.Filter(events)))
	return nil
}
