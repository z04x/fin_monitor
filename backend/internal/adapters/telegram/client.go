// Package telegram is a minimal Bot API adapter: one method, no SDK.
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	botToken   string
	httpClient *http.Client
}

func NewClient(botToken string) *Client {
	return &Client{
		botToken:   botToken,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// SendMessage posts plain text (no parse_mode) so digest text needs no
// Markdown/HTML escaping.
//
// Retries on transient network errors (TLS/TCP resets are common on flaky
// routes to api.telegram.org) and on 429/5xx, with a short backoff.
func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.botToken)
	form := url.Values{
		"chat_id": {chatID},
		"text":    {text},
	}.Encode()

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt-1) * time.Second)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form))
		if err != nil {
			return c.redact(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// *url.Error stringifies the full request URL, which embeds the
			// bot token — strip it before the error goes anywhere near logs.
			lastErr = c.redact(err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("telegram status %d", resp.StatusCode)
			continue
		}

		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			var body struct {
				Description string `json:"description"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			return fmt.Errorf("telegram status %d: %s", resp.StatusCode, body.Description)
		}
		return nil
	}
	return fmt.Errorf("telegram send failed after %d attempts: %w", maxAttempts, lastErr)
}

func (c *Client) redact(err error) error {
	if err == nil || c.botToken == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), c.botToken, "[REDACTED]"))
}
