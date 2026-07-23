// Package digest builds the morning Telegram message from a day's
// earnings-calendar events. Pure domain logic: no HTTP, no provider JSON.
package digest

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Event struct {
	Ticker          string
	CompanyName     string
	Hour            string // "bmo" | "amc" | "" (unknown / TBD)
	EPSEstimate     *float64
	EPSActual       *float64
	RevenueEstimate *float64
}

// HasEstimates drops tickers with no analyst coverage at all (closed-end
// funds, illiquid names) — both estimates null.
func HasEstimates(e Event) bool {
	return e.EPSEstimate != nil || e.RevenueEstimate != nil
}

func Filter(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		if HasEstimates(e) {
			out = append(out, e)
		}
	}
	return out
}

func timeTag(hour string) string {
	switch hour {
	case "bmo":
		return "BMO"
	case "amc":
		return "AMC"
	default:
		return "TBD"
	}
}

func sizeTag(revenueEstimate *float64) string {
	if revenueEstimate == nil {
		return "S"
	}
	switch {
	case *revenueEstimate >= 10_000_000_000:
		return "XL"
	case *revenueEstimate >= 1_000_000_000:
		return "L"
	case *revenueEstimate >= 100_000_000:
		return "M"
	default:
		return "S"
	}
}

func signalTags(e Event) []string {
	var tags []string
	lossExpected := e.EPSEstimate != nil && *e.EPSEstimate < 0
	if lossExpected {
		tags = append(tags, "LOSS-EXPECTED", "TURNAROUND-WATCH")
	}
	if e.EPSActual != nil {
		switch {
		case lossExpected && *e.EPSActual > 0:
			tags = append(tags, "TURNAROUND")
		case e.EPSEstimate != nil && *e.EPSActual > *e.EPSEstimate:
			tags = append(tags, "BEAT")
		case e.EPSEstimate != nil && *e.EPSActual < *e.EPSEstimate:
			tags = append(tags, "MISS")
		}
	}
	return tags
}

func formatMoney(v *float64) string {
	if v == nil {
		return "н/д"
	}
	switch abs := *v; {
	case abs >= 1_000_000_000 || abs <= -1_000_000_000:
		return fmt.Sprintf("$%.1fB", *v/1_000_000_000)
	case abs >= 1_000_000 || abs <= -1_000_000:
		return fmt.Sprintf("$%.1fM", *v/1_000_000)
	default:
		return fmt.Sprintf("$%.0f", *v)
	}
}

func formatEPS(v *float64) string {
	if v == nil {
		return "н/д"
	}
	return fmt.Sprintf("%.2f", *v)
}

var ruWeekday = map[time.Weekday]string{
	time.Monday:    "понедельник",
	time.Tuesday:   "вторник",
	time.Wednesday: "среда",
	time.Thursday:  "четверг",
	time.Friday:    "пятница",
	time.Saturday:  "суббота",
	time.Sunday:    "воскресенье",
}

var ruMonth = map[time.Month]string{
	time.January:   "января",
	time.February:  "февраля",
	time.March:     "марта",
	time.April:     "апреля",
	time.May:       "мая",
	time.June:      "июня",
	time.July:      "июля",
	time.August:    "августа",
	time.September: "сентября",
	time.October:   "октября",
	time.November:  "ноября",
	time.December:  "декабря",
}

func formatDateHeader(day time.Time) string {
	return fmt.Sprintf("%s [%d %s %d]", ruWeekday[day.Weekday()], day.Day(), ruMonth[day.Month()], day.Year())
}

func formatEvent(e Event) string {
	tags := append([]string{sizeTag(e.RevenueEstimate), timeTag(e.Hour)}, signalTags(e)...)
	tagLine := ""
	for _, t := range tags {
		tagLine += "[" + t + "] "
	}
	tagLine = strings.TrimSpace(tagLine)

	name := e.CompanyName
	if name == "" {
		name = e.Ticker
	}

	return fmt.Sprintf(
		"%s\n%s — %s\nEPS ожидание: %s | Revenue ожидание: %s",
		tagLine, e.Ticker, name, formatEPS(e.EPSEstimate), formatMoney(e.RevenueEstimate),
	)
}

// Build renders the full digest text for one day. Events must already be
// mapped from the provider response; Build filters, groups by hour and
// sorts by revenue estimate (largest first) within each group.
func Build(day time.Time, events []Event) string {
	header := fmt.Sprintf("📅 Отчёты на %s", formatDateHeader(day))

	filtered := Filter(events)
	if len(filtered) == 0 {
		return header + "\n\nСегодня отчётов по значимым тикерам нет."
	}

	groups := []struct {
		title string
		hour  string
	}{
		{"До открытия (BMO)", "bmo"},
		{"После закрытия (AMC)", "amc"},
		{"Время не указано (TBD)", ""},
	}

	var sections []string
	for _, group := range groups {
		var bucket []Event
		for _, e := range filtered {
			if e.Hour == group.hour {
				bucket = append(bucket, e)
			}
		}
		if len(bucket) == 0 {
			continue
		}
		sort.SliceStable(bucket, func(i, j int) bool {
			ri, rj := bucket[i].RevenueEstimate, bucket[j].RevenueEstimate
			if ri == nil {
				return false
			}
			if rj == nil {
				return true
			}
			return *ri > *rj
		})

		lines := make([]string, 0, len(bucket))
		for _, e := range bucket {
			lines = append(lines, formatEvent(e))
		}
		sections = append(sections, fmt.Sprintf("── %s ──\n\n%s", group.title, strings.Join(lines, "\n\n")))
	}

	return header + "\n\n" + strings.Join(sections, "\n\n")
}

// TelegramMessageLimit is the Bot API's max text length per sendMessage call.
const TelegramMessageLimit = 4096

// Chunk splits text into pieces no longer than limit, breaking only on
// paragraph boundaries ("\n\n") so an event's tag/name/EPS lines never get
// torn apart mid-entry. A continuation marker is added to every chunk after
// the first so recipients can tell the digest was split.
func Chunk(text string, limit int) []string {
	if len(text) <= limit {
		return []string{text}
	}

	const continued = "(продолжение)\n\n"
	paragraphs := strings.Split(text, "\n\n")

	var chunks []string
	current := ""
	startNext := func() string {
		if len(chunks) > 0 {
			return continued
		}
		return ""
	}

	for _, p := range paragraphs {
		candidate := p
		if current != "" {
			candidate = current + "\n\n" + p
		} else {
			candidate = startNext() + p
		}
		if len(candidate) > limit && current != "" {
			chunks = append(chunks, current)
			current = ""
			candidate = startNext() + p
		}
		current = candidate
	}
	if current != "" {
		chunks = append(chunks, current)
	}

	return chunks
}
