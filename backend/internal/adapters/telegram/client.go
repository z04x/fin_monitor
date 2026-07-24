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

// Update is the slice of a Telegram webhook payload we care about: the chat
// id (to scope who we answer) and the message text (to parse a command).
type Update struct {
	Message *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

// ParseCommand extracts (chatID, command, args) from a raw webhook body.
// command is lower-cased without the leading slash and without any
// "@botname" suffix (Telegram appends it in groups); args is the rest of the
// text, trimmed. ok is false if the body isn't a parseable /command message.
func ParseCommand(body []byte) (chatID int64, command, args string, ok bool) {
	var u Update
	if err := json.Unmarshal(body, &u); err != nil || u.Message == nil {
		return 0, "", "", false
	}
	text := strings.TrimSpace(u.Message.Text)
	if !strings.HasPrefix(text, "/") {
		return 0, "", "", false
	}

	head, rest, _ := strings.Cut(text, " ")
	command = strings.ToLower(strings.TrimPrefix(head, "/"))
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	if command == "" {
		return 0, "", "", false
	}
	return u.Message.Chat.ID, command, strings.TrimSpace(rest), true
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
