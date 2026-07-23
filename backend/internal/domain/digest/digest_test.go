package digest

import (
	"strings"
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }

func TestFilter(t *testing.T) {
	events := []Event{
		{Ticker: "AAA"},                                    // no estimates at all -> dropped
		{Ticker: "BBB", EPSEstimate: f(1.2)},                // eps only -> kept
		{Ticker: "CCC", RevenueEstimate: f(1_000_000)},      // revenue only -> kept
		{Ticker: "DDD", EPSEstimate: f(0), RevenueEstimate: f(0)},
	}

	got := Filter(events)
	if len(got) != 3 {
		t.Fatalf("expected 3 events after filter, got %d", len(got))
	}
}

func TestSignalTags(t *testing.T) {
	cases := []struct {
		name string
		e    Event
		want []string
	}{
		{"no signal", Event{EPSEstimate: f(1.0)}, nil},
		{"loss expected", Event{EPSEstimate: f(-0.5)}, []string{"LOSS-EXPECTED", "TURNAROUND-WATCH"}},
		{"realized turnaround", Event{EPSEstimate: f(-0.5), EPSActual: f(0.1)}, []string{"LOSS-EXPECTED", "TURNAROUND-WATCH", "TURNAROUND"}},
		{"beat", Event{EPSEstimate: f(1.0), EPSActual: f(1.5)}, []string{"BEAT"}},
		{"miss", Event{EPSEstimate: f(1.0), EPSActual: f(0.5)}, []string{"MISS"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := signalTags(tc.e)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestSizeTag(t *testing.T) {
	cases := []struct {
		revenue *float64
		want    string
	}{
		{nil, "S"},
		{f(50_000_000), "S"},
		{f(500_000_000), "M"},
		{f(5_000_000_000), "L"},
		{f(50_000_000_000), "XL"},
	}
	for _, tc := range cases {
		if got := sizeTag(tc.revenue); got != tc.want {
			t.Errorf("sizeTag(%v) = %q, want %q", tc.revenue, got, tc.want)
		}
	}
}

func TestBuild_NoEvents(t *testing.T) {
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	text := Build(day, nil)
	if !strings.Contains(text, "отчётов по значимым тикерам нет") {
		t.Fatalf("expected empty-day message, got: %s", text)
	}
}

func TestBuild_GroupsAndSortsWithinBucket(t *testing.T) {
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	events := []Event{
		{Ticker: "SMALL", Hour: "bmo", RevenueEstimate: f(1_000_000)},
		{Ticker: "BIG", Hour: "bmo", RevenueEstimate: f(20_000_000_000)},
		{Ticker: "NOTIME", RevenueEstimate: f(1_000_000)},
	}

	text := Build(day, events)
	bigIdx := strings.Index(text, "BIG")
	smallIdx := strings.Index(text, "SMALL")
	if bigIdx == -1 || smallIdx == -1 || bigIdx > smallIdx {
		t.Fatalf("expected BIG before SMALL within BMO bucket, got:\n%s", text)
	}
	if !strings.Contains(text, "TBD") {
		t.Fatalf("expected TBD section for event with no hour, got:\n%s", text)
	}
}

func TestChunk_UnderLimitReturnsSingleChunk(t *testing.T) {
	got := Chunk("short text", 4096)
	if len(got) != 1 || got[0] != "short text" {
		t.Fatalf("expected single unmodified chunk, got %v", got)
	}
}

func TestChunk_SplitsOnParagraphBoundaries(t *testing.T) {
	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	events := make([]Event, 0, 100)
	for i := 0; i < 100; i++ {
		events = append(events, Event{Ticker: "TICK", CompanyName: "Some Long Company Name Inc", Hour: "bmo", RevenueEstimate: f(1_000_000_000)})
	}
	text := Build(day, events)

	chunks := Chunk(text, 500)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > 500 {
			t.Errorf("chunk %d exceeds limit: %d chars", i, len(c))
		}
		if i > 0 && !strings.HasPrefix(c, "(продолжение)") {
			t.Errorf("chunk %d missing continuation marker:\n%s", i, c)
		}
	}
	if !strings.Contains(strings.Join(chunks, "\n"), "TICK") {
		t.Fatalf("expected ticker content preserved across chunks")
	}
}
