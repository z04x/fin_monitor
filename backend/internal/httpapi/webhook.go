package httpapi

import (
	"context"
	"crypto/subtle"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"market-analyzer/backend/internal/adapters/finnhub"
	"market-analyzer/backend/internal/adapters/telegram"
	"market-analyzer/backend/internal/config"
	"market-analyzer/backend/internal/domain/report"
)

// handleTelegramWebhook answers /reports TICKER commands. It verifies
// Telegram's secret-token header, scopes replies to the configured chat, and
// does the (slow, throttled) provider work in a goroutine so Telegram gets an
// immediate 200 and never retries the same update.
func handleTelegramWebhook(cfg config.Config, deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Constant-time secret check. If no secret is configured we skip the
		// check (local dev), but production must set TELEGRAM_WEBHOOK_SECRET.
		if cfg.TelegramWebhookSecret != "" {
			got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
			if subtle.ConstantTimeCompare([]byte(got), []byte(cfg.TelegramWebhookSecret)) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusOK) // ack anyway; nothing to retry
			return
		}

		chatID, command, args, ok := telegram.ParseCommand(body)
		// Always 200 so Telegram stops redelivering; we just don't act on
		// non-commands, other commands, or messages from other chats.
		w.WriteHeader(http.StatusOK)
		if !ok || command != "reports" {
			return
		}
		if cfg.TelegramChatID != "" && strconv.FormatInt(chatID, 10) != cfg.TelegramChatID {
			slog.Warn("ignoring /reports from unexpected chat", "chat_id", chatID)
			return
		}

		years, ticker, ok := parseReportsArgs(args)
		if !ok {
			go sendText(cfg, deps, chatID, "Использование: /reports [1|2] ТИКЕР\nНапример: /reports MU или /reports 2 AAPL")
			return
		}

		go processReports(cfg, deps, chatID, ticker, years)
	}
}

// parseReportsArgs reads an optional leading depth (1 or 2 years) followed by
// a ticker. "MU" -> (1, "MU"); "2 AAPL" -> (2, "AAPL"); "1 mu" -> (1, "MU").
func parseReportsArgs(args string) (years int, ticker string, ok bool) {
	fields := strings.Fields(args)
	years = 1
	switch len(fields) {
	case 1:
		ticker = fields[0]
	case 2:
		switch fields[0] {
		case "1":
			years = 1
		case "2":
			years = 2
		default:
			return 0, "", false
		}
		ticker = fields[1]
	default:
		return 0, "", false
	}
	return years, strings.ToUpper(ticker), true
}

// processReports fetches profile + earnings history and sends the card. Runs
// in its own goroutine with a fresh context (the request context is already
// cancelled once we returned 200).
func processReports(cfg config.Config, deps Deps, chatID int64, ticker string, years int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	profile, err := deps.Finnhub.GetProfile(ctx, ticker)
	if err != nil {
		slog.Error("reports: profile failed", "ticker", ticker, "error", err)
		sendText(cfg, deps, chatID, "Не удалось получить данные по «"+ticker+"». Проверь тикер.")
		return
	}
	if profile.Name == "" {
		sendText(cfg, deps, chatID, "Тикер «"+ticker+"» не найден.")
		return
	}

	history, err := deps.Finnhub.GetEarnings(ctx, ticker)
	if err != nil {
		slog.Error("reports: earnings failed", "ticker", ticker, "error", err)
		// Still send the header card; history just won't be there.
		history = nil
	}

	// Metrics are a nice-to-have: on failure we log and omit the block rather
	// than failing the whole card.
	var metrics *report.Metrics
	if m, err := deps.Finnhub.GetMetrics(ctx, ticker); err != nil {
		slog.Warn("reports: metrics failed", "ticker", ticker, "error", err)
	} else {
		metrics = mapmetrics(m)
	}

	card := report.Card{
		Ticker:    ticker,
		Name:      profile.Name,
		Industry:  profile.Industry,
		MarketCap: profile.MarketCapitalization,
		Metrics:   metrics,
		Past:      maphistory(history, years*4),
	}
	sendText(cfg, deps, chatID, report.Build(card))
}

// maphistory converts provider rows to domain quarters, newest-first, capped
// to limit. stock/earnings already returns newest-first.
func maphistory(history []finnhub.EarningHistory, limit int) []report.Quarter {
	if len(history) > limit {
		history = history[:limit]
	}
	out := make([]report.Quarter, 0, len(history))
	for _, h := range history {
		out = append(out, report.Quarter{
			Year:            h.Year,
			Quarter:         h.Quarter,
			EPSEstimate:     h.EPSEstimate,
			EPSActual:       h.EPSActual,
			SurprisePercent: h.SurprisePercent,
		})
	}
	return out
}

func mapmetrics(m finnhub.Metrics) *report.Metrics {
	return &report.Metrics{
		PE:               m.PE,
		ROE:              m.ROE,
		GrossMargin:      m.GrossMargin,
		OperatingMargin:  m.OperatingMargin,
		NetMargin:        m.NetMargin,
		RevenueGrowthYoY: m.RevenueGrowthYoY,
		Week52High:       m.Week52High,
		Week52Low:        m.Week52Low,
	}
}

func sendText(cfg config.Config, deps Deps, chatID int64, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := deps.Telegram.SendMessage(ctx, strconv.FormatInt(chatID, 10), text); err != nil {
		slog.Error("reports: send failed", "chat_id", chatID, "error", err)
	}
}
