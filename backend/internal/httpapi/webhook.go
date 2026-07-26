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
	"market-analyzer/backend/internal/domain/reaction"
	"market-analyzer/backend/internal/domain/report"
	"market-analyzer/backend/internal/domain/sentiment"
	"market-analyzer/backend/internal/prices"
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	card := report.Card{
		Ticker:    ticker,
		Name:      profile.Name,
		Industry:  profile.Industry,
		MarketCap: profile.MarketCapitalization,
	}

	// Metrics — nice-to-have: on failure log and omit the block.
	if m, err := deps.Finnhub.GetMetrics(ctx, ticker); err != nil {
		slog.Warn("reports: metrics failed", "ticker", ticker, "error", err)
	} else {
		card.Metrics = mapmetrics(m)
	}

	// Analyst sentiment — optional block.
	if recs, err := deps.Finnhub.GetRecommendation(ctx, ticker); err != nil {
		slog.Warn("reports: recommendation failed", "ticker", ticker, "error", err)
	} else if s, ok := sentiment.Summarize(mapRecs(recs)); ok {
		card.Sentiment = &s
	}

	// EPS surprise history (last ~4q) — matched by index onto recent events.
	earnings, err := deps.Finnhub.GetEarnings(ctx, ticker)
	if err != nil {
		slog.Warn("reports: earnings failed", "ticker", ticker, "error", err)
		earnings = nil
	}

	// Price reaction — the core. Announcement dates (EDGAR) × daily prices
	// (Tiingo, stock + SPY). Any failure here degrades to a card without the
	// reaction sections rather than failing the whole command.
	addReaction(ctx, deps, ticker, years, earnings, &card)

	sendText(cfg, deps, chatID, report.Build(card))
}

// addReaction fills card.Stats / card.Events / card.StatsFrom from EDGAR
// announcements and Tiingo prices. Best-effort: logs and returns on any error.
func addReaction(ctx context.Context, deps Deps, ticker string, years int, earnings []finnhub.EarningHistory, card *report.Card) {
	anns, err := deps.Edgar.GetAnnouncements(ctx, ticker)
	if err != nil || len(anns) == 0 {
		if err != nil {
			slog.Warn("reports: edgar failed", "ticker", ticker, "error", err)
		}
		return
	}

	// Prices from ~120 days before the earliest announcement (covers the
	// sigma/pre-drift windows) through today, for the ticker and SPY.
	earliest := anns[len(anns)-1].Date
	from := earliest.AddDate(0, 0, -120).Format("2006-01-02")
	today := time.Now().UTC().Format("2006-01-02")

	stockP, err := deps.Prices.GetDailyPrices(ctx, ticker, from, today)
	if err != nil {
		slog.Warn("reports: stock prices failed", "ticker", ticker, "error", err)
		return
	}
	spyP, err := deps.Prices.GetDailyPrices(ctx, "SPY", from, today)
	if err != nil {
		slog.Warn("reports: SPY prices failed", "error", err)
		return
	}
	stockBars, spyBars := mapBars(stockP), mapBars(spyP)
	reaction.SortBars(stockBars)
	reaction.SortBars(spyBars)

	// Compute reaction for every announcement (aggregates use all history);
	// keep a parallel slice so recent events can attach their own reaction.
	perAnn := make([]*reaction.Event, len(anns))
	var all []reaction.Event
	for i, a := range anns {
		ev, err := reaction.PerEvent(stockBars, spyBars, a.Date, a.Session)
		if err != nil {
			continue
		}
		e := ev
		perAnn[i] = &e
		all = append(all, ev)
	}
	if len(all) > 0 {
		stats := reaction.Aggregate(all)
		card.Stats = &stats
		card.StatsFrom = earliest.Year()
	}

	// Recent events shown in detail (years*4), with surprise attached by index
	// to the Finnhub quarters (newest-first on both sides).
	show := years * 4
	if show > len(anns) {
		show = len(anns)
	}
	for i := 0; i < show; i++ {
		e := report.Event{Date: anns[i].Date, Session: anns[i].Session, Reaction: perAnn[i]}
		if i < len(earnings) {
			q := earnings[i]
			e.Year, e.Quarter = q.Year, q.Quarter
			e.EPSEstimate, e.EPSActual, e.SurprisePercent = q.EPSEstimate, q.EPSActual, q.SurprisePercent
		}
		card.Events = append(card.Events, e)
	}
}

// mapBars converts Tiingo daily rows to reaction bars using the ADJUSTED
// series (adjusted-only, per spec §1 — raw would distort history on splits).
// Rows missing an adjusted close are skipped.
func mapBars(rows []prices.DailyPrice) []reaction.Bar {
	out := make([]reaction.Bar, 0, len(rows))
	for _, r := range rows {
		if r.AdjClose == nil || len(r.Date) < 10 {
			continue
		}
		date, err := time.Parse("2006-01-02", r.Date[:10])
		if err != nil {
			continue
		}
		b := reaction.Bar{Date: date, Close: *r.AdjClose}
		if r.AdjOpen != nil {
			b.Open = *r.AdjOpen
		}
		if r.AdjHigh != nil {
			b.High = *r.AdjHigh
		}
		if r.AdjLow != nil {
			b.Low = *r.AdjLow
		}
		if r.AdjVolume != nil {
			b.Volume = float64(*r.AdjVolume)
		}
		out = append(out, b)
	}
	return out
}

func mapRecs(recs []finnhub.Recommendation) []sentiment.Snapshot {
	out := make([]sentiment.Snapshot, 0, len(recs))
	for _, r := range recs {
		out = append(out, sentiment.Snapshot{
			Period: r.Period, StrongBuy: r.StrongBuy, Buy: r.Buy,
			Hold: r.Hold, Sell: r.Sell, StrongSell: r.StrongSell,
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
