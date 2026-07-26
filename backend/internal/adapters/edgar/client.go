// Package edgar reads earnings-announcement dates from SEC EDGAR: the
// filingDate of each 8-K carrying item 2.02 ("Results of Operations"), plus a
// BMO/AMC classification from the filing's acceptanceDateTime. Free, no key —
// only a descriptive User-Agent (with contact email) is required by the SEC.
package edgar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	tickersURL     = "https://www.sec.gov/files/company_tickers.json"
	submissionsURL = "https://data.sec.gov/submissions/CIK%s.json"
)

// Announcement is one earnings release: the filing (announcement) date and
// the market session it landed in.
type Announcement struct {
	Date    time.Time
	Session string // "amc" | "bmo" | "" (unknown / mid-session)
}

type Client struct {
	userAgent  string
	httpClient *http.Client
	et         *time.Location

	mu  sync.Mutex
	cik map[string]string // ticker (upper) -> 10-digit zero-padded CIK
}

func NewClient(userAgent string) *Client {
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		et = time.UTC // fall back; session classification degrades to UTC hours
	}
	return &Client{
		userAgent:  userAgent,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		et:         et,
	}
}

// GetAnnouncements returns every 8-K item-2.02 announcement for a ticker,
// newest-first.
func (c *Client) GetAnnouncements(ctx context.Context, ticker string) ([]Announcement, error) {
	cik, err := c.cikFor(ctx, ticker)
	if err != nil {
		return nil, err
	}

	var subs submissions
	if err := c.getJSON(ctx, fmt.Sprintf(submissionsURL, cik), &subs); err != nil {
		return nil, fmt.Errorf("edgar submissions %s: %w", ticker, err)
	}

	r := subs.Filings.Recent
	out := make([]Announcement, 0, 40)
	for i := range r.Form {
		if r.Form[i] != "8-K" || !strings.Contains(r.Items[i], "2.02") {
			continue
		}
		date, err := time.Parse("2006-01-02", r.FilingDate[i])
		if err != nil {
			continue
		}
		out = append(out, Announcement{
			Date:    date,
			Session: c.classify(r.AcceptanceDateTime[i]),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

// classify maps an acceptanceDateTime (RFC3339 UTC) to bmo/amc via ET session
// bounds. Empty/unparseable → "" (caller uses the safe both-sides window).
func (c *Client) classify(acceptance string) string {
	t, err := time.Parse(time.RFC3339, acceptance)
	if err != nil {
		return ""
	}
	et := t.In(c.et)
	mins := et.Hour()*60 + et.Minute()
	const open, close = 9*60 + 30, 16 * 60
	switch {
	case mins <= open:
		return "bmo"
	case mins >= close:
		return "amc"
	default:
		return ""
	}
}

func (c *Client) cikFor(ctx context.Context, ticker string) (string, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	c.mu.Lock()
	cached := c.cik
	c.mu.Unlock()

	if cached == nil {
		loaded, err := c.loadCIKMap(ctx)
		if err != nil {
			return "", err
		}
		c.mu.Lock()
		c.cik = loaded
		cached = loaded
		c.mu.Unlock()
	}

	cik, ok := cached[ticker]
	if !ok {
		return "", fmt.Errorf("edgar: ticker %q not found in CIK map", ticker)
	}
	return cik, nil
}

func (c *Client) loadCIKMap(ctx context.Context) (map[string]string, error) {
	var raw map[string]struct {
		CIK    int    `json:"cik_str"`
		Ticker string `json:"ticker"`
	}
	if err := c.getJSON(ctx, tickersURL, &raw); err != nil {
		return nil, fmt.Errorf("edgar CIK map: %w", err)
	}
	m := make(map[string]string, len(raw))
	for _, e := range raw {
		m[strings.ToUpper(e.Ticker)] = fmt.Sprintf("%010d", e.CIK)
	}
	return m, nil
}

func (c *Client) getJSON(ctx context.Context, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// SEC blocks requests without a descriptive User-Agent. Leave
	// Accept-Encoding unset so Go's transport negotiates + decompresses gzip
	// transparently (setting it manually disables auto-decompression).
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("edgar status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

type submissions struct {
	Filings struct {
		Recent struct {
			Form               []string `json:"form"`
			Items              []string `json:"items"`
			FilingDate         []string `json:"filingDate"`
			AcceptanceDateTime []string `json:"acceptanceDateTime"`
		} `json:"recent"`
	} `json:"filings"`
}
