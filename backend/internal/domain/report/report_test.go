package report

import (
	"strings"
	"testing"
	"time"

	"market-analyzer/backend/internal/domain/reaction"
	"market-analyzer/backend/internal/domain/sentiment"
)

func f(v float64) *float64 { return &v }

func dt(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestBuildHeaderAndMetrics(t *testing.T) {
	c := Card{
		Ticker:    "MU",
		Name:      "Micron Technology",
		Industry:  "Semiconductors",
		MarketCap: f(120_000), // $120B in millions
		Metrics: &Metrics{
			PE: f(21.0), ROE: f(70.6), GrossMargin: f(72.6), OperatingMargin: f(65.6),
			NetMargin: f(55.9), RevenueGrowthYoY: f(167.0), Week52High: f(1255), Week52Low: f(103.38),
		},
	}
	got := Build(c)
	for _, want := range []string{
		"MU — Micron Technology", "Semiconductors", "$120.0B",
		"P/E: 21.0 · ROE: 70.6%", "52-нед. диапазон: $103.38 – $1255.00",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildSentiment(t *testing.T) {
	c := Card{
		Ticker:    "MU",
		Sentiment: &sentiment.Summary{Label: "Покупать", Score: 1.21, Total: 56, ScoreTrend: 0.02, CoverageDelta: 4},
	}
	got := Build(c)
	if !strings.Contains(got, "👥 Аналитики: Покупать (score +1.21, 56 аналитиков)") {
		t.Fatalf("sentiment line missing:\n%s", got)
	}
	if !strings.Contains(got, "консенсус стабилен") || !strings.Contains(got, "покрытие +4") {
		t.Fatalf("trend line wrong:\n%s", got)
	}
}

func TestBuildStats(t *testing.T) {
	c := Card{
		Ticker: "MU",
		Stats: &reaction.Stats{
			N: 37, ExpMove: 8.5, StdMove: 6, MinReaction: -22, MaxReaction: 18, WinRate: 0.6,
			AvgPreDrift: 2.1, HasPreDrift: true, AvgContinue: 1.4, HasPostDrift: true,
			AvgVolumeX: 2.8, HasVolume: true, GapUpRate: 0.65, GapGoRate: 0.58, HasGap: true,
		},
		StatsFrom: 2017,
	}
	got := Build(c)
	for _, want := range []string{
		"📉 Реакция на отчёты (37 отчётов с 2017):",
		"Ожидаемый ход: ±8.5% vs рынка (от -22.0% до +18.0%)",
		"Растёт: 60% случаев",
		"Разгон до отчёта: +2.1% за 3 дня",
		"После отчёта: продолжает +1.4% (3 дня)",
		"Гэп: вверх 65%, удерживает 58%",
		"Объём в день реакции: ×2.8 от обычного",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildEventSellTheNews(t *testing.T) {
	c := Card{
		Ticker: "MU",
		Events: []Event{
			{
				Date: dt("2026-06-24"), Session: "amc", Year: 2026, Quarter: 3,
				EPSActual: f(25.11), SurprisePercent: f(17.3),
				Reaction: &reaction.Event{
					ReactionVsSpy: -12.2, Sigma: -3.0, VolumeX: 3.2,
					Gap: &reaction.Gap{Direction: "down", Pattern: "full-fade"},
				},
			},
		},
	}
	got := Build(c)
	if !strings.Contains(got, "24 июн 2026 (AMC) · beat +17.3%") {
		t.Fatalf("event head wrong:\n%s", got)
	}
	if !strings.Contains(got, "реакция -12.2% vs рынка -3.0σ · гэп вниз, полный фейд · ×3.2 объём") {
		t.Fatalf("event detail wrong:\n%s", got)
	}
	if !strings.Contains(got, "⚠️ sell-the-news") {
		t.Fatalf("expected sell-the-news flag:\n%s", got)
	}
}

func TestBuildEventBeatAndRunNoFlag(t *testing.T) {
	c := Card{
		Ticker: "MU",
		Events: []Event{
			{
				Date: dt("2026-06-24"), Session: "amc", SurprisePercent: f(17.3), EPSActual: f(25.11),
				Reaction: &reaction.Event{ReactionVsSpy: 15.7, Sigma: 3.5, VolumeX: 3.2,
					Gap: &reaction.Gap{Direction: "up", Pattern: "go"}},
			},
		},
	}
	got := Build(c)
	if strings.Contains(got, "sell-the-news") {
		t.Fatalf("beat-and-run should not flag sell-the-news:\n%s", got)
	}
	if !strings.Contains(got, "гэп вверх, удерживает") {
		t.Fatalf("gap words wrong:\n%s", got)
	}
}

func TestBuildFallsBackToTicker(t *testing.T) {
	got := Build(Card{Ticker: "XYZ"})
	if !strings.Contains(got, "XYZ — XYZ") {
		t.Fatalf("ticker fallback missing:\n%s", got)
	}
}
