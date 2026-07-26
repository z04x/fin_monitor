package reaction

import "math"

// Stats summarises a ticker's earnings-reaction behaviour over its history.
// All percentage fields are market-neutral (vs SPY) unless noted.
type Stats struct {
	N           int     // events counted
	ExpMove     float64 // mean |reaction vs SPY|, the typical earnings move
	StdMove     float64 // stddev of reaction vs SPY (dispersion)
	MinReaction float64 // most negative reaction vs SPY
	MaxReaction float64 // most positive reaction vs SPY
	WinRate     float64 // fraction of events with reaction vs SPY > 0

	AvgPreDrift  float64 // mean run-up into the print (vs SPY); NaN-safe 0 if none
	HasPreDrift  bool
	AvgContinue  float64 // mean post-drift aligned with reaction sign (>0 = momentum)
	HasPostDrift bool

	AvgVolumeX float64 // mean reaction-day volume multiple
	HasVolume  bool

	GapUpRate float64 // among events with a gap, fraction that gapped up
	GapGoRate float64 // among events with a gap, fraction that held (pattern "go")
	HasGap    bool
}

// Aggregate reduces per-event reactions to summary statistics. Events with
// missing sub-metrics are simply skipped for that metric (denominators track
// availability), so a short/edge history degrades gracefully.
func Aggregate(events []Event) Stats {
	var s Stats
	if len(events) == 0 {
		return s
	}

	var sumAbs, sumSigned float64
	s.MinReaction = math.Inf(1)
	s.MaxReaction = math.Inf(-1)
	var wins int

	var sumPre float64
	var nPre int
	var sumCont float64
	var nCont int
	var sumVol float64
	var nVol int
	var nGap, gapUp, gapGo int

	for _, e := range events {
		r := e.ReactionVsSpy
		sumAbs += math.Abs(r)
		sumSigned += r
		if r < s.MinReaction {
			s.MinReaction = r
		}
		if r > s.MaxReaction {
			s.MaxReaction = r
		}
		if r > 0 {
			wins++
		}
		s.N++

		if e.PreDrift != nil {
			sumPre += *e.PreDrift
			nPre++
		}
		if e.PostDrift != nil {
			// Align with reaction direction: positive => continued (momentum).
			sumCont += *e.PostDrift * sign(r)
			nCont++
		}
		if e.VolumeX > 0 {
			sumVol += e.VolumeX
			nVol++
		}
		if e.Gap != nil {
			nGap++
			if e.Gap.Direction == "up" {
				gapUp++
			}
			if e.Gap.Pattern == "go" {
				gapGo++
			}
		}
	}

	if s.N == 0 {
		return Stats{}
	}
	n := float64(s.N)
	s.ExpMove = sumAbs / n
	mean := sumSigned / n
	var ss float64
	for _, e := range events {
		diff := e.ReactionVsSpy - mean
		ss += diff * diff
	}
	s.StdMove = math.Sqrt(ss / n)
	s.WinRate = float64(wins) / n

	if nPre > 0 {
		s.AvgPreDrift = sumPre / float64(nPre)
		s.HasPreDrift = true
	}
	if nCont > 0 {
		s.AvgContinue = sumCont / float64(nCont)
		s.HasPostDrift = true
	}
	if nVol > 0 {
		s.AvgVolumeX = sumVol / float64(nVol)
		s.HasVolume = true
	}
	if nGap > 0 {
		s.GapUpRate = float64(gapUp) / float64(nGap)
		s.GapGoRate = float64(gapGo) / float64(nGap)
		s.HasGap = true
	}
	return s
}

func sign(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}
