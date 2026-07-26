package reaction

import (
	"math"
	"testing"
	"time"
)

func d(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func bar(date string, o, h, l, c, v float64) Bar {
	return Bar{Date: d(date), Open: o, High: h, Low: l, Close: c, Volume: v}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestPerEventAMCGapAndGo(t *testing.T) {
	stock := []Bar{
		bar("2026-06-22", 99, 101, 98, 100, 1000),
		bar("2026-06-23", 100, 101, 99, 100, 1000),
		bar("2026-06-24", 100, 101, 99, 100, 1000),  // D, announcement (AMC)
		bar("2026-06-25", 105, 115, 104, 112, 3000), // D+1 reaction day
	}
	spy := []Bar{
		bar("2026-06-24", 0, 0, 0, 400, 0),
		bar("2026-06-25", 0, 0, 0, 404, 0),
	}
	ev, err := PerEvent(stock, spy, d("2026-06-24"), "amc")
	if err != nil {
		t.Fatal(err)
	}
	if !approx(ev.ReactionPct, 12) {
		t.Fatalf("reaction=%v want 12", ev.ReactionPct)
	}
	if !approx(ev.SpyPct, 1) || !approx(ev.ReactionVsSpy, 11) {
		t.Fatalf("spy=%v vsSpy=%v want 1/11", ev.SpyPct, ev.ReactionVsSpy)
	}
	if ev.Gap == nil {
		t.Fatal("expected gap")
	}
	if !approx(ev.Gap.Pct, 5) || ev.Gap.Direction != "up" || ev.Gap.Pattern != "go" {
		t.Fatalf("gap=%+v want pct5/up/go", *ev.Gap)
	}
	// closePos = (112-104)/(115-104) = 8/11
	if !approx(ev.Gap.ClosePos, 8.0/11.0) {
		t.Fatalf("closePos=%v want %v", ev.Gap.ClosePos, 8.0/11.0)
	}
	// volumeX = 3000 / avg(1000,1000,1000) = 3
	if !approx(ev.VolumeX, 3) {
		t.Fatalf("volumeX=%v want 3", ev.VolumeX)
	}
}

func TestPerEventBMO(t *testing.T) {
	stock := []Bar{
		bar("2026-06-23", 100, 101, 99, 100, 1000), // D-1
		bar("2026-06-24", 96, 99, 95, 98, 1000),    // D = annDate (BMO): before=D-1, after=D
	}
	ev, err := PerEvent(stock, nil, d("2026-06-24"), "bmo")
	if err != nil {
		t.Fatal(err)
	}
	if !approx(ev.ReactionPct, -2) {
		t.Fatalf("reaction=%v want -2", ev.ReactionPct)
	}
	// gap: prevClose=100, open=96 -> -4%; down; close98 > prevClose100? no, 98<100 -> close<prevClose and >open(96): partial-fade
	if ev.Gap.Direction != "down" || ev.Gap.Pattern != "partial-fade" {
		t.Fatalf("gap=%+v want down/partial-fade", *ev.Gap)
	}
}

func TestPerEventUnknownNoGap(t *testing.T) {
	stock := []Bar{
		bar("2026-06-23", 100, 101, 99, 100, 1000),  // before (D-1)
		bar("2026-06-24", 100, 101, 99, 100, 1000),  // anchor
		bar("2026-06-25", 100, 110, 100, 108, 1000), // after (D+1)
	}
	ev, err := PerEvent(stock, nil, d("2026-06-24"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !approx(ev.ReactionPct, 8) {
		t.Fatalf("reaction=%v want 8", ev.ReactionPct)
	}
	if ev.Gap != nil {
		t.Fatal("unknown session should have no gap")
	}
}

func TestPerEventFullFadeDown(t *testing.T) {
	// gap down but recovers back above prior close -> full-fade
	stock := []Bar{
		bar("2026-06-24", 100, 101, 99, 100, 1000), // D (amc), before
		bar("2026-06-25", 95, 103, 94, 101, 1000),  // D+1: open95<100 (down gap), close101>100 -> full-fade
	}
	ev, err := PerEvent(stock, nil, d("2026-06-24"), "amc")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Gap.Direction != "down" || ev.Gap.Pattern != "full-fade" {
		t.Fatalf("gap=%+v want down/full-fade", *ev.Gap)
	}
}

func TestPerEventNoDataAtEdge(t *testing.T) {
	stock := []Bar{
		bar("2026-06-24", 100, 101, 99, 100, 1000), // D, amc needs D+1 which is missing
	}
	if _, err := PerEvent(stock, nil, d("2026-06-24"), "amc"); err != ErrNoData {
		t.Fatalf("want ErrNoData, got %v", err)
	}
	// annDate before all bars
	if _, err := PerEvent(stock, nil, d("2020-01-01"), "amc"); err != ErrNoData {
		t.Fatalf("want ErrNoData for early date, got %v", err)
	}
}

func TestSigma(t *testing.T) {
	// Build a series where daily returns alternate to give a known-ish stddev,
	// then a big reaction, and check sigma = reaction / dailyStd.
	stock := []Bar{}
	price := 100.0
	dates := []string{"2026-01-01", "2026-01-02", "2026-01-03", "2026-01-04", "2026-01-05", "2026-01-06"}
	closes := []float64{100, 101, 100, 101, 100, 100} // ~±1% daily wobble up to before-idx
	for i, ds := range dates {
		price = closes[i]
		stock = append(stock, bar(ds, price, price+1, price-1, price, 1000))
	}
	// announcement AMC on 2026-01-06 needs a next day; add reaction day +10%
	stock = append(stock, bar("2026-01-07", 100, 112, 100, 110, 1000)) // +10%
	ev, err := PerEvent(stock, nil, d("2026-01-06"), "amc")
	if err != nil {
		t.Fatal(err)
	}
	if !approx(ev.ReactionPct, 10) {
		t.Fatalf("reaction=%v want 10", ev.ReactionPct)
	}
	if ev.Sigma <= 0 {
		t.Fatalf("expected positive sigma, got %v", ev.Sigma)
	}
	// daily wobble ~1%, so a 10% move should be several sigma
	if ev.Sigma < 5 {
		t.Fatalf("expected sigma >~5 for 10%% move on ~1%% vol, got %v", ev.Sigma)
	}
}
