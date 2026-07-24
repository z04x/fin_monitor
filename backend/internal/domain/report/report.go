// Package report builds the text of a /reports TICKER Telegram card:
// company header + beat/miss history. Pure domain logic — no HTTP, no
// provider JSON. Price reaction is intentionally out of scope for now: the
// free Finnhub tier no longer serves historical report dates (see spec §6
// and the /reports design notes), which the reaction calc requires.
package report

import (
	"fmt"
	"strings"

	"market-analyzer/backend/internal/domain/digest"
)

// Quarter is one past earnings row (Finnhub stock/earnings). Period is the
// fiscal quarter END, not the publication date — shown only as a Q/Y label.
type Quarter struct {
	Year            int
	Quarter         int
	EPSEstimate     *float64
	EPSActual       *float64
	SurprisePercent *float64
}

// Metrics is the optional valuation/quality block (Finnhub stock/metric TTM
// fields). Any field may be nil; the block is omitted entirely if empty.
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

// Card is the fully-resolved input to Build.
type Card struct {
	Ticker    string
	Name      string
	Industry  string
	MarketCap *float64 // in millions (Finnhub marketCapitalization unit)
	Metrics   *Metrics // nil to omit the metrics block
	Past      []Quarter
}

// beatTag returns the beat/miss label for a quarter, or "" if it can't be
// determined (no actual yet, or no surprise figure).
func beatTag(q Quarter) string {
	if q.EPSActual == nil {
		return ""
	}
	if q.SurprisePercent != nil {
		switch {
		case *q.SurprisePercent > 0:
			return fmt.Sprintf("BEAT +%.1f%%", *q.SurprisePercent)
		case *q.SurprisePercent < 0:
			return fmt.Sprintf("MISS %.1f%%", *q.SurprisePercent)
		default:
			return "В ЛИНИЮ"
		}
	}
	if q.EPSEstimate != nil {
		switch {
		case *q.EPSActual > *q.EPSEstimate:
			return "BEAT"
		case *q.EPSActual < *q.EPSEstimate:
			return "MISS"
		default:
			return "В ЛИНИЮ"
		}
	}
	return ""
}

// formatMarketCap renders Finnhub's marketCapitalization (in $millions).
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

// formatMetrics renders the metrics block, or "" if there's nothing to show.
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

func formatQuarter(q Quarter) string {
	label := fmt.Sprintf("Q%d %d", q.Quarter, q.Year)
	if tag := beatTag(q); tag != "" {
		label += " [" + tag + "]"
	}
	return fmt.Sprintf(
		"%s\nEPS: %s (ожидание %s)",
		label, digest.FormatEPS(q.EPSActual), digest.FormatEPS(q.EPSEstimate),
	)
}

// Build renders the card text. Past is expected already trimmed to the
// requested depth and ordered newest-first by the caller.
func Build(c Card) string {
	name := c.Name
	if name == "" {
		name = c.Ticker
	}
	header := fmt.Sprintf("📊 %s — %s", c.Ticker, name)

	var meta []string
	if c.Industry != "" {
		meta = append(meta, c.Industry)
	}
	if cap := formatMarketCap(c.MarketCap); cap != "" {
		meta = append(meta, "капитализация "+cap)
	}
	if len(meta) > 0 {
		header += "\n" + strings.Join(meta, " · ")
	}

	if block := formatMetrics(c.Metrics); block != "" {
		header += "\n\n" + block
	}

	if len(c.Past) == 0 {
		return header + "\n\nИстории отчётов по этому тикеру нет."
	}

	lines := make([]string, 0, len(c.Past))
	for _, q := range c.Past {
		lines = append(lines, formatQuarter(q))
	}

	return header + fmt.Sprintf("\n\nКак отчитывался (посл. %d кв.):\n\n", len(c.Past)) +
		strings.Join(lines, "\n\n")
}
