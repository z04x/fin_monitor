// Package finnhub implements the finnhub.io adapter used by both the
// Telegram digest bot (cmd/bot) and, later, the site's daily pipeline.
package finnhub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Free tier allows 60 req/min; pace at 1 req/sec so a single caller never
// has to think about the limit.
const minRequestInterval = time.Second

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client

	mu       sync.Mutex
	lastCall time.Time
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) GetCalendar(ctx context.Context, from, to string) ([]CalendarEvent, error) {
	var resp EarningsCalendarResponse
	query := url.Values{"from": {from}, "to": {to}}
	if err := c.get(ctx, "/calendar/earnings", query, &resp); err != nil {
		return nil, fmt.Errorf("get calendar: %w", err)
	}
	return resp.EarningsCalendar, nil
}

// GetEarnings returns the beat/miss history for a symbol (stock/earnings),
// newest quarter first. Available on the free tier (unlike historical
// calendar/earnings ranges, which the free tier limits to the current week).
func (c *Client) GetEarnings(ctx context.Context, symbol string) ([]EarningHistory, error) {
	var history []EarningHistory
	query := url.Values{"symbol": {symbol}}
	if err := c.get(ctx, "/stock/earnings", query, &history); err != nil {
		return nil, fmt.Errorf("get earnings %s: %w", symbol, err)
	}
	return history, nil
}

func (c *Client) GetProfile(ctx context.Context, symbol string) (Profile, error) {
	var profile Profile
	query := url.Values{"symbol": {symbol}}
	if err := c.get(ctx, "/stock/profile2", query, &profile); err != nil {
		return Profile{}, fmt.Errorf("get profile %s: %w", symbol, err)
	}
	return profile, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, target any) error {
	if query == nil {
		query = url.Values{}
	}
	query.Set("token", c.token)
	endpoint := c.baseURL + path + "?" + query.Encode()

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		c.throttle()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return c.redact(err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// *url.Error stringifies the full request URL, which embeds the
			// token as a query param — strip it before the error goes
			// anywhere near logs.
			return c.redact(err)
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("finnhub status %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("finnhub status %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return lastErr
}

func (c *Client) redact(err error) error {
	if err == nil || c.token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), c.token, "[REDACTED]"))
}

func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()

	wait := minRequestInterval - time.Since(c.lastCall)
	if wait > 0 {
		time.Sleep(wait)
	}
	c.lastCall = time.Now()
}
