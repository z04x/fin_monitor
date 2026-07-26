package sentiment

import (
	"math"
	"testing"
)

func TestSummarize(t *testing.T) {
	// newest-first
	snaps := []Snapshot{
		{Period: "2026-07-01", StrongBuy: 18, Buy: 33, Hold: 4, Sell: 1, StrongSell: 0}, // total 56
		{Period: "2026-06-01", StrongBuy: 18, Buy: 33, Hold: 3, Sell: 1, StrongSell: 0}, // 55
		{Period: "2026-04-01", StrongBuy: 17, Buy: 31, Hold: 3, Sell: 1, StrongSell: 0}, // 52
	}
	s, ok := Summarize(snaps)
	if !ok {
		t.Fatal("expected ok")
	}
	if s.Total != 56 {
		t.Fatalf("total=%d want 56", s.Total)
	}
	// score latest = (2*18+33-1-0)/56 = (36+33-1)/56 = 68/56 = 1.214
	if math.Abs(s.Score-68.0/56.0) > 1e-6 {
		t.Fatalf("score=%v", s.Score)
	}
	if s.Label != "Покупать" {
		t.Fatalf("label=%q", s.Label)
	}
	// coverage grew 52 -> 56
	if s.CoverageDelta != 4 {
		t.Fatalf("coverageDelta=%d want 4", s.CoverageDelta)
	}
	// trend: latest - oldest, both ~1.2 -> small
	oldest := 66.0 / 52.0 // (2*17+31-1)/52 = (34+31-1)/52 = 64/52
	_ = oldest
}

func TestSummarizeLabels(t *testing.T) {
	cases := []struct {
		sb, b, h, se, ss int
		want             string
	}{
		{20, 0, 0, 0, 0, "Активно покупать"},  // score 2
		{0, 10, 0, 0, 0, "Покупать"},          // score 1
		{0, 0, 10, 0, 0, "Держать"},           // score 0
		{0, 0, 0, 10, 0, "Продавать"},         // score -1
		{0, 0, 0, 0, 10, "Активно продавать"}, // score -2
	}
	for _, c := range cases {
		s, _ := Summarize([]Snapshot{{StrongBuy: c.sb, Buy: c.b, Hold: c.h, Sell: c.se, StrongSell: c.ss}})
		if s.Label != c.want {
			t.Fatalf("label=%q want %q", s.Label, c.want)
		}
	}
}

func TestSummarizeEmpty(t *testing.T) {
	if _, ok := Summarize(nil); ok {
		t.Fatal("nil should not be ok")
	}
	if _, ok := Summarize([]Snapshot{{}}); ok {
		t.Fatal("zero-coverage should not be ok")
	}
}
