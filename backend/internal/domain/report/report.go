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

// Card is the fully-resolved input to Build.
type Card struct {
	Ticker    string
	Name      string
	Industry  string
	MarketCap *float64 // in millions (Finnhub marketCapitalization unit)
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
