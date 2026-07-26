package reaction

import (
	"math"
	"testing"
)

func p(v float64) *float64 { return &v }

func TestAggregate(t *testing.T) {
	events := []Event{
		{ReactionVsSpy: 10, PreDrift: p(2), PostDrift: p(3), VolumeX: 2, Gap: &Gap{Direction: "up", Pattern: "go"}},
		{ReactionVsSpy: -6, PreDrift: p(-1), PostDrift: p(2), VolumeX: 4, Gap: &Gap{Direction: "down", Pattern: "full-fade"}},
		{ReactionVsSpy: 4, PreDrift: p(0), PostDrift: p(-1), VolumeX: 3, Gap: &Gap{Direction: "up", Pattern: "partial-fade"}},
	}
	s := Aggregate(events)

	if s.N != 3 {
		t.Fatalf("N=%d want 3", s.N)
	}
	// ExpMove = (10+6+4)/3 = 6.6667
	if !approx(s.ExpMove, 20.0/3.0) {
		t.Fatalf("ExpMove=%v", s.ExpMove)
	}
	if !approx(s.MinReaction, -6) || !approx(s.MaxReaction, 10) {
		t.Fatalf("min=%v max=%v", s.MinReaction, s.MaxReaction)
	}
	// wins: 10,4 -> 2/3
	if !approx(s.WinRate, 2.0/3.0) {
		t.Fatalf("winRate=%v", s.WinRate)
	}
	// PreDrift mean = (2-1+0)/3 = 0.3333
	if !s.HasPreDrift || !approx(s.AvgPreDrift, 1.0/3.0) {
		t.Fatalf("avgPre=%v", s.AvgPreDrift)
	}
	// Continuation = post*sign(reaction): 3*+1 + 2*-1 + (-1)*+1 = 3-2-1=0 -> /3 = 0
	if !s.HasPostDrift || !approx(s.AvgContinue, 0) {
		t.Fatalf("avgContinue=%v want 0", s.AvgContinue)
	}
	// VolumeX mean = (2+4+3)/3 = 3
	if !s.HasVolume || !approx(s.AvgVolumeX, 3) {
		t.Fatalf("avgVol=%v", s.AvgVolumeX)
	}
	// GapUpRate = 2/3, GapGoRate = 1/3
	if !s.HasGap || !approx(s.GapUpRate, 2.0/3.0) || !approx(s.GapGoRate, 1.0/3.0) {
		t.Fatalf("gapUp=%v gapGo=%v", s.GapUpRate, s.GapGoRate)
	}
}

func TestAggregateEmpty(t *testing.T) {
	s := Aggregate(nil)
	if s.N != 0 || s.HasGap || s.HasPreDrift {
		t.Fatalf("empty aggregate should be zero-valued, got %+v", s)
	}
}

func TestAggregatePartialAvailability(t *testing.T) {
	// Some events lack drift/gap — denominators must track availability.
	events := []Event{
		{ReactionVsSpy: 5, VolumeX: 2},                  // no drift, no gap
		{ReactionVsSpy: -3, PreDrift: p(1), VolumeX: 0}, // pre only, zero vol skipped
	}
	s := Aggregate(events)
	if s.HasGap {
		t.Fatal("no gaps present")
	}
	if !s.HasPreDrift || !approx(s.AvgPreDrift, 1) {
		t.Fatalf("avgPre=%v want 1 (single sample)", s.AvgPreDrift)
	}
	// only first event has VolumeX>0
	if !s.HasVolume || !approx(s.AvgVolumeX, 2) {
		t.Fatalf("avgVol=%v want 2", s.AvgVolumeX)
	}
	if math.IsNaN(s.ExpMove) {
		t.Fatal("ExpMove NaN")
	}
}
