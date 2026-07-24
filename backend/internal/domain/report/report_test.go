package report

import (
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }

func TestBeatTag(t *testing.T) {
	cases := []struct {
		name string
		q    Quarter
		want string
	}{
		{"no actual yet", Quarter{EPSEstimate: f(1.0)}, ""},
		{"beat by surprise", Quarter{EPSActual: f(1.5), SurprisePercent: f(12.3)}, "BEAT +12.3%"},
		{"miss by surprise", Quarter{EPSActual: f(0.5), SurprisePercent: f(-8.0)}, "MISS -8.0%"},
		{"inline by surprise", Quarter{EPSActual: f(1.0), SurprisePercent: f(0)}, "В ЛИНИЮ"},
		{"beat without surprise", Quarter{EPSEstimate: f(1.0), EPSActual: f(1.2)}, "BEAT"},
		{"miss without surprise", Quarter{EPSEstimate: f(1.0), EPSActual: f(0.8)}, "MISS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := beatTag(tc.q); got != tc.want {
				t.Fatalf("beatTag = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildEmptyHistory(t *testing.T) {
	got := Build(Card{Ticker: "AAA", Name: "Alpha", Past: nil})
	if !strings.Contains(got, "Истории отчётов") {
		t.Fatalf("expected empty-history notice, got:\n%s", got)
	}
}

func TestBuildFull(t *testing.T) {
	c := Card{
		Ticker:    "MU",
		Name:      "Micron Technology",
		Industry:  "Technology",
		MarketCap: f(120_000), // $120B, given in millions
		Past: []Quarter{
			{Year: 2026, Quarter: 3, EPSEstimate: f(21.40), EPSActual: f(25.11), SurprisePercent: f(17.3)},
			{Year: 2026, Quarter: 2, EPSEstimate: f(9.58), EPSActual: f(12.20), SurprisePercent: f(27.3)},
		},
	}
	got := Build(c)

	for _, want := range []string{
		"MU — Micron Technology",
		"Technology",
		"$120.0B",
		"Q3 2026 [BEAT +17.3%]",
		"EPS: 25.11 (ожидание 21.40)",
		"Q2 2026 [BEAT +27.3%]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q, got:\n%s", want, got)
		}
	}
}

func TestBuildFallsBackToTickerWhenNoName(t *testing.T) {
	got := Build(Card{Ticker: "XYZ", Past: []Quarter{{Year: 2025, Quarter: 1, EPSActual: f(1), EPSEstimate: f(1), SurprisePercent: f(0)}}})
	if !strings.Contains(got, "XYZ — XYZ") {
		t.Fatalf("expected ticker fallback in header, got:\n%s", got)
	}
}
