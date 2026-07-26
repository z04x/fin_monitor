// Package sentiment turns analyst-consensus snapshots into a compact view:
// current stance (weighted score + label) and its short trend. Pure domain
// logic — plain numbers in, summary out.
package sentiment

// Snapshot is one monthly consensus count.
type Snapshot struct {
	Period     string
	StrongBuy  int
	Buy        int
	Hold       int
	Sell       int
	StrongSell int
}

func (s Snapshot) total() int {
	return s.StrongBuy + s.Buy + s.Hold + s.Sell + s.StrongSell
}

// score is the coverage-normalised consensus, strongBuy=+2 … strongSell=-2.
func (s Snapshot) score() (float64, bool) {
	t := s.total()
	if t == 0 {
		return 0, false
	}
	weighted := 2*s.StrongBuy + s.Buy - s.Sell - 2*s.StrongSell
	return float64(weighted) / float64(t), true
}

// Summary is the rendered-ready sentiment view.
type Summary struct {
	Latest        Snapshot
	Total         int
	Score         float64
	Label         string  // plain-language stance
	ScoreTrend    float64 // latest.score - oldest.score (over the window)
	CoverageDelta int     // latest total - oldest total
}

func label(score float64) string {
	switch {
	case score >= 1.5:
		return "Активно покупать"
	case score >= 0.5:
		return "Покупать"
	case score > -0.5:
		return "Держать"
	case score > -1.5:
		return "Продавать"
	default:
		return "Активно продавать"
	}
}

// Summarize reduces snapshots (newest-first, as Finnhub returns them) to a
// Summary. Returns false if there's nothing usable.
func Summarize(snaps []Snapshot) (Summary, bool) {
	// Find the newest snapshot with coverage.
	var latest *Snapshot
	for i := range snaps {
		if snaps[i].total() > 0 {
			latest = &snaps[i]
			break
		}
	}
	if latest == nil {
		return Summary{}, false
	}
	sc, _ := latest.score()

	sum := Summary{
		Latest: *latest,
		Total:  latest.total(),
		Score:  sc,
		Label:  label(sc),
	}

	// Oldest snapshot with coverage, for the trend.
	for i := len(snaps) - 1; i >= 0; i-- {
		if snaps[i].total() > 0 {
			if os, ok := snaps[i].score(); ok {
				sum.ScoreTrend = sc - os
				sum.CoverageDelta = latest.total() - snaps[i].total()
			}
			break
		}
	}
	return sum, true
}
