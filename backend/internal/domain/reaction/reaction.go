// Package reaction computes how a stock's price behaves around an earnings
// announcement — per event and aggregated over history. Pure domain logic:
// takes plain daily bars (adjusted OHLCV) in, returns numbers out. No HTTP,
// no provider types. Daily granularity only (intraday data isn't free).
package reaction

import (
	"errors"
	"math"
	"sort"
	"time"
)

// Bar is one daily session, adjusted series (adjOpen/High/Low/Close, adjVolume).
type Bar struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// Gap describes the open of the reaction day relative to the prior close and
// how the day resolved.
type Gap struct {
	Pct         float64 // gap size: open[reaction] / close[before] - 1, in %
	Direction   string  // "up" | "down"
	Pattern     string  // "go" (held/extended) | "partial-fade" | "full-fade"
	IntradayPct float64 // open->close of the reaction day, in %
	ClosePos    float64 // (close-low)/(high-low) on the reaction day, 0..1
}

// Event is the computed reaction for one announcement.
type Event struct {
	Date          time.Time
	Session       string  // "bmo" | "amc" | ""
	ReactionPct   float64 // close-to-close over the session window
	SpyPct        float64
	ReactionVsSpy float64
	Sigma         float64  // reaction in units of the stock's daily-return stddev
	VolumeX       float64  // reaction-day volume / 20-session average before it
	Gap           *Gap     // nil for unknown session or missing intraday inputs
	PreDrift      *float64 // 3-session run-up into the print, vs SPY (nil if off-history)
	PostDrift     *float64 // 3-session drift after the reaction day, vs SPY (nil if off-history)
}

const driftDays = 3

var ErrNoData = errors.New("reaction: insufficient price data for window")

const (
	sigmaWindow  = 60
	volumeWindow = 20
)

// PerEvent computes the reaction for one announcement. Bars must be sorted
// oldest-first. Returns ErrNoData when the window falls off the available
// history (caller omits the event rather than guessing).
func PerEvent(stock, spy []Bar, annDate time.Time, session string) (Event, error) {
	if len(stock) == 0 {
		return Event{}, ErrNoData
	}
	anchor := lastIdxOnOrBefore(stock, annDate)
	if anchor < 0 {
		return Event{}, ErrNoData
	}

	var beforeIdx, afterIdx int
	switch session {
	case "amc":
		beforeIdx, afterIdx = anchor, anchor+1
	case "bmo":
		beforeIdx, afterIdx = anchor-1, anchor
	default: // unknown: span both sides
		beforeIdx, afterIdx = anchor-1, anchor+1
	}
	if beforeIdx < 0 || afterIdx >= len(stock) {
		return Event{}, ErrNoData
	}

	before, after := stock[beforeIdx], stock[afterIdx]
	if before.Close == 0 {
		return Event{}, ErrNoData
	}
	reactionPct := (after.Close - before.Close) / before.Close * 100

	ev := Event{
		Date:        annDate,
		Session:     session,
		ReactionPct: reactionPct,
	}

	// Market-neutral: SPY over the exact same dates.
	spyByDate := indexByDate(spy)
	if sb, ok := spyByDate[before.Date]; ok {
		if sa, ok := spyByDate[after.Date]; ok && sb.Close != 0 {
			ev.SpyPct = (sa.Close - sb.Close) / sb.Close * 100
		}
	}
	ev.ReactionVsSpy = ev.ReactionPct - ev.SpyPct

	// Sigma: daily-return stddev over the window ending at beforeIdx.
	if sd := dailyReturnStd(stock, beforeIdx, sigmaWindow); sd > 0 {
		ev.Sigma = reactionPct / sd
	}

	// Volume confirmation: reaction-day volume vs prior 20-session average.
	if avg := avgVolume(stock, afterIdx, volumeWindow); avg > 0 {
		ev.VolumeX = after.Volume / avg
	}

	// Gap detail: only meaningful when a single reaction day is defined
	// (bmo/amc), not the two-sided unknown window.
	if session == "amc" || session == "bmo" {
		ev.Gap = computeGap(before.Close, after)
	}

	// Pre-earnings run-up: stock vs SPY over the driftDays sessions into the
	// print (ending at beforeIdx).
	ev.PreDrift = driftVsSpy(stock, spyByDate, beforeIdx-driftDays, beforeIdx)
	// Post-earnings drift (PEAD): stock vs SPY over the driftDays sessions
	// after the reaction day.
	ev.PostDrift = driftVsSpy(stock, spyByDate, afterIdx, afterIdx+driftDays)

	return ev, nil
}

// driftVsSpy returns the stock's return from fromIdx to toIdx minus SPY's over
// the same dates, in %, or nil if indices fall outside history.
func driftVsSpy(stock []Bar, spyByDate map[time.Time]Bar, fromIdx, toIdx int) *float64 {
	if fromIdx < 0 || toIdx >= len(stock) || fromIdx == toIdx {
		return nil
	}
	from, to := stock[fromIdx], stock[toIdx]
	if from.Close == 0 {
		return nil
	}
	stockRet := (to.Close - from.Close) / from.Close * 100
	var spyRet float64
	if sf, ok := spyByDate[from.Date]; ok {
		if st, ok := spyByDate[to.Date]; ok && sf.Close != 0 {
			spyRet = (st.Close - sf.Close) / sf.Close * 100
		}
	}
	v := stockRet - spyRet
	return &v
}

func computeGap(prevClose float64, day Bar) *Gap {
	if prevClose == 0 || day.Open == 0 {
		return nil
	}
	g := &Gap{
		Pct:         (day.Open/prevClose - 1) * 100,
		IntradayPct: (day.Close - day.Open) / day.Open * 100,
	}
	if day.Open >= prevClose {
		g.Direction = "up"
		switch {
		case day.Close >= day.Open:
			g.Pattern = "go"
		case day.Close > prevClose:
			g.Pattern = "partial-fade"
		default:
			g.Pattern = "full-fade"
		}
	} else {
		g.Direction = "down"
		switch {
		case day.Close <= day.Open:
			g.Pattern = "go"
		case day.Close < prevClose:
			g.Pattern = "partial-fade"
		default:
			g.Pattern = "full-fade"
		}
	}
	if rng := day.High - day.Low; rng > 0 {
		g.ClosePos = (day.Close - day.Low) / rng
	}
	return g
}

// lastIdxOnOrBefore returns the largest index whose Date <= target, or -1.
// Assumes bars sorted oldest-first.
func lastIdxOnOrBefore(bars []Bar, target time.Time) int {
	lo, hi := 0, len(bars)
	for lo < hi {
		mid := (lo + hi) / 2
		if bars[mid].Date.After(target) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo - 1
}

func indexByDate(bars []Bar) map[time.Time]Bar {
	m := make(map[time.Time]Bar, len(bars))
	for _, b := range bars {
		m[b.Date] = b
	}
	return m
}

// dailyReturnStd is the population stddev of simple daily returns (in %) over
// up to n sessions ending at endIdx (inclusive).
func dailyReturnStd(bars []Bar, endIdx, n int) float64 {
	start := endIdx - n
	if start < 1 {
		start = 1
	}
	var rets []float64
	for i := start; i <= endIdx; i++ {
		p := bars[i-1].Close
		if p == 0 {
			continue
		}
		rets = append(rets, (bars[i].Close-p)/p*100)
	}
	if len(rets) < 2 {
		return 0
	}
	var mean float64
	for _, r := range rets {
		mean += r
	}
	mean /= float64(len(rets))
	var ss float64
	for _, r := range rets {
		ss += (r - mean) * (r - mean)
	}
	return math.Sqrt(ss / float64(len(rets)))
}

// avgVolume averages volume over the n sessions immediately before endIdx.
func avgVolume(bars []Bar, endIdx, n int) float64 {
	start := endIdx - n
	if start < 0 {
		start = 0
	}
	var sum float64
	var cnt int
	for i := start; i < endIdx; i++ {
		sum += bars[i].Volume
		cnt++
	}
	if cnt == 0 {
		return 0
	}
	return sum / float64(cnt)
}

// ensure sort import used (bars are expected pre-sorted, but expose a helper
// so callers can guarantee it).
func SortBars(bars []Bar) {
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date.Before(bars[j].Date) })
}
