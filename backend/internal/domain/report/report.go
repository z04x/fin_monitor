// Package report builds the text of a /reports TICKER Telegram card: company
// header, TTM metrics, analyst sentiment, and — the core — how the stock has
// historically reacted to earnings (aggregate stats + recent events). Pure
// domain logic: plain values and other domain types in, text out. No HTTP, no
// provider JSON. Daily granularity (intraday isn't free).
package report

import (
	"fmt"
	"strings"
	"time"

	"market-analyzer/backend/internal/domain/digest"
	"market-analyzer/backend/internal/domain/reaction"
	"market-analyzer/backend/internal/domain/sentiment"
)

// Metrics is the optional valuation/quality block (Finnhub stock/metric TTM).
type Metrics struct {
	PE               *float64
	ROE              *float64 // percent
	GrossMargin      *float64 // percent
	OperatingMargin  *float64 // percent
	NetMargin        *float64 // percent
	RevenueGrowthYoY *float64 // percent
	Week52High       *float64
	Week52Low        *float64
}

// Event is one earnings event shown in detail: its date/session, the EPS
// surprise (when matched to a Finnhub quarter), and the computed price
// reaction (nil if it couldn't be computed off available history).
type Event struct {
	Date            time.Time
	Session         string // "bmo" | "amc" | ""
	Year, Quarter   int    // 0 if not matched to a Finnhub quarter
	EPSEstimate     *float64
	EPSActual       *float64
	SurprisePercent *float64
	Reaction        *reaction.Event
}

// Card is the fully-resolved input to Build. Any pointer/slice field left
// empty is simply omitted from the output.
type Card struct {
	Ticker    string
	Name      string
	Industry  string
	MarketCap *float64 // Finnhub marketCapitalization unit (millions)
	Metrics   *Metrics
	Sentiment *sentiment.Summary
	Stats     *reaction.Stats // aggregate reaction over full history
	StatsFrom int             // earliest year covered by Stats (0 to omit)
	Events    []Event         // recent events, newest-first
}

// --- number formatting -------------------------------------------------------

func formatMarketCap(millions *float64) string {
	if millions == nil {
		return ""
	}
	v := *millions * 1_000_000
	return digest.FormatMoney(&v)
}

func ratio(v *float64) string {
	if v == nil {
		return "н/д"
	}
	return fmt.Sprintf("%.1f", *v)
}

func percent(v *float64) string {
	if v == nil {
		return "н/д"
	}
	return fmt.Sprintf("%.1f%%", *v)
}

func signedPercent(v *float64) string {
	if v == nil {
		return "н/д"
	}
	return fmt.Sprintf("%+.1f%%", *v)
}

func price(v *float64) string {
	if v == nil {
		return "н/д"
	}
	return fmt.Sprintf("$%.2f", *v)
}

// --- sections ----------------------------------------------------------------

func header(c Card) string {
	name := c.Name
	if name == "" {
		name = c.Ticker
	}
	h := fmt.Sprintf("📊 %s — %s", c.Ticker, name)

	var meta []string
	if c.Industry != "" {
		meta = append(meta, c.Industry)
	}
	if cap := formatMarketCap(c.MarketCap); cap != "" {
		meta = append(meta, "капитализация "+cap)
	}
	if len(meta) > 0 {
		h += "\n" + strings.Join(meta, " · ")
	}
	return h
}

func formatMetrics(m *Metrics) string {
	if m == nil {
		return ""
	}
	var lines []string
	if m.PE != nil || m.ROE != nil {
		lines = append(lines, fmt.Sprintf("P/E: %s · ROE: %s", ratio(m.PE), percent(m.ROE)))
	}
	if m.GrossMargin != nil || m.OperatingMargin != nil || m.NetMargin != nil {
		lines = append(lines, fmt.Sprintf("Маржа: вал. %s / опер. %s / чист. %s",
			percent(m.GrossMargin), percent(m.OperatingMargin), percent(m.NetMargin)))
	}
	if m.RevenueGrowthYoY != nil {
		lines = append(lines, "Рост выручки г/г: "+signedPercent(m.RevenueGrowthYoY))
	}
	if m.Week52Low != nil || m.Week52High != nil {
		lines = append(lines, fmt.Sprintf("52-нед. диапазон: %s – %s", price(m.Week52Low), price(m.Week52High)))
	}
	if len(lines) == 0 {
		return ""
	}
	return "📈 Метрики (TTM):\n" + strings.Join(lines, "\n")
}

func formatSentiment(s *sentiment.Summary) string {
	if s == nil {
		return ""
	}
	line := fmt.Sprintf("👥 Аналитики: %s (score %+.2f, %d аналитиков)", s.Label, s.Score, s.Total)

	var trend []string
	switch {
	case s.ScoreTrend > 0.15:
		trend = append(trend, "консенсус улучшается")
	case s.ScoreTrend < -0.15:
		trend = append(trend, "консенсус ухудшается")
	default:
		trend = append(trend, "консенсус стабилен")
	}
	if s.CoverageDelta > 0 {
		trend = append(trend, fmt.Sprintf("покрытие +%d", s.CoverageDelta))
	} else if s.CoverageDelta < 0 {
		trend = append(trend, fmt.Sprintf("покрытие %d", s.CoverageDelta))
	}
	return line + "\n" + "За 4 мес: " + strings.Join(trend, ", ")
}

func formatStats(s *reaction.Stats, from int) string {
	if s == nil || s.N == 0 {
		return ""
	}
	title := fmt.Sprintf("📉 Реакция на отчёты (%d отчётов", s.N)
	if from > 0 {
		title += fmt.Sprintf(" с %d", from)
	}
	title += "):"

	lines := []string{
		fmt.Sprintf("Ожидаемый ход: ±%.1f%% vs рынка (от %+.1f%% до %+.1f%%)", s.ExpMove, s.MinReaction, s.MaxReaction),
		fmt.Sprintf("Растёт: %.0f%% случаев", s.WinRate*100),
	}
	if s.HasPreDrift {
		lines = append(lines, fmt.Sprintf("Разгон до отчёта: %+.1f%% за 3 дня", s.AvgPreDrift))
	}
	if s.HasPostDrift {
		word := "продолжает"
		if s.AvgContinue < 0 {
			word = "откатывает"
		}
		lines = append(lines, fmt.Sprintf("После отчёта: %s %+.1f%% (3 дня)", word, s.AvgContinue))
	}
	if s.HasGap {
		lines = append(lines, fmt.Sprintf("Гэп: вверх %.0f%%, удерживает %.0f%%", s.GapUpRate*100, s.GapGoRate*100))
	}
	if s.HasVolume {
		lines = append(lines, fmt.Sprintf("Объём в день реакции: ×%.1f от обычного", s.AvgVolumeX))
	}
	return title + "\n" + strings.Join(lines, "\n")
}

func gapWords(g *reaction.Gap) string {
	if g == nil {
		return ""
	}
	dir := "вверх"
	if g.Direction == "down" {
		dir = "вниз"
	}
	var pat string
	switch g.Pattern {
	case "go":
		pat = "удерживает"
	case "partial-fade":
		pat = "частичный фейд"
	case "full-fade":
		pat = "полный фейд"
	}
	if pat == "" {
		return "гэп " + dir
	}
	return "гэп " + dir + ", " + pat
}

func surpriseTag(e Event) string {
	if e.SurprisePercent == nil || e.EPSActual == nil {
		return ""
	}
	switch {
	case *e.SurprisePercent > 0:
		return fmt.Sprintf("beat +%.1f%%", *e.SurprisePercent)
	case *e.SurprisePercent < 0:
		return fmt.Sprintf("miss %.1f%%", *e.SurprisePercent)
	default:
		return "в линию"
	}
}

var ruMonthShort = map[time.Month]string{
	time.January: "янв", time.February: "фев", time.March: "мар", time.April: "апр",
	time.May: "мая", time.June: "июн", time.July: "июл", time.August: "авг",
	time.September: "сен", time.October: "окт", time.November: "ноя", time.December: "дек",
}

func sessionTag(s string) string {
	switch s {
	case "amc":
		return "AMC"
	case "bmo":
		return "BMO"
	default:
		return ""
	}
}

func formatEvent(e Event) string {
	date := fmt.Sprintf("%d %s %d", e.Date.Day(), ruMonthShort[e.Date.Month()], e.Date.Year())
	head := date
	if st := sessionTag(e.Session); st != "" {
		head += " (" + st + ")"
	}
	if tag := surpriseTag(e); tag != "" {
		head += " · " + tag
	}

	if e.Reaction == nil {
		return head
	}
	r := e.Reaction

	parts := []string{fmt.Sprintf("реакция %+.1f%% vs рынка", r.ReactionVsSpy)}
	if r.Sigma != 0 {
		parts = append(parts, fmt.Sprintf("%+.1fσ", r.Sigma))
	}
	detail := strings.Join(parts, " ")
	if gw := gapWords(r.Gap); gw != "" {
		detail += " · " + gw
	}
	if r.VolumeX > 0 {
		detail += fmt.Sprintf(" · ×%.1f объём", r.VolumeX)
	}

	// Sell-the-news: beat but the market sold it (reaction vs SPY negative).
	if e.SurprisePercent != nil && *e.SurprisePercent > 0 && r.ReactionVsSpy < 0 {
		detail += "  ⚠️ sell-the-news"
	}

	return head + "\n  " + detail
}

// Build renders the full card. Sections with no data are omitted.
func Build(c Card) string {
	sections := []string{header(c)}

	if m := formatMetrics(c.Metrics); m != "" {
		sections = append(sections, m)
	}
	if s := formatSentiment(c.Sentiment); s != "" {
		sections = append(sections, s)
	}
	if s := formatStats(c.Stats, c.StatsFrom); s != "" {
		sections = append(sections, s)
	}
	if len(c.Events) > 0 {
		lines := make([]string, 0, len(c.Events))
		for _, e := range c.Events {
			lines = append(lines, formatEvent(e))
		}
		sections = append(sections, "🗓 Последние отчёты:\n"+strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}
