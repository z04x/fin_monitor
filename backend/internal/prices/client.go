package prices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
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

// GetDailyPrices returns daily OHLCV (incl. adjusted series) for a ticker
// over [startDate, endDate] (YYYY-MM-DD), oldest-first. Works for SPY too.
func (c *Client) GetDailyPrices(ctx context.Context, ticker, startDate, endDate string) ([]DailyPrice, error) {
	var prices []DailyPrice
	path := "/tiingo/daily/" + url.PathEscape(ticker) + "/prices"
	query := url.Values{"startDate": {startDate}, "endDate": {endDate}}
	if err := c.get(ctx, path, query, &prices); err != nil {
		return nil, fmt.Errorf("get daily prices %s: %w", ticker, err)
	}
	return prices, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, target any) error {
	if query == nil {
		query = url.Values{}
	}
	query.Set("token", c.token)

	endpoint := c.baseURL + path + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tiingo status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
