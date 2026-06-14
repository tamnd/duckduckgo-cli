// Package duckduckgo is the library behind the duckduckgo command line:
// the HTTP client, request shaping, and the typed data models for the
// DuckDuckGo Instant Answer API.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package duckduckgo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DefaultUserAgent identifies the client to DuckDuckGo.
const DefaultUserAgent = "duckduckgo-cli/0.1 (tamnd87@gmail.com)"

// Host is the site this client talks to, and the host the URI driver in
// domain.go claims.
const Host = "api.duckduckgo.com"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Client talks to the DuckDuckGo Instant Answer API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 10 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      500 * time.Millisecond,
		Retries:   3,
	}
}

// wireResult is the JSON shape the DuckDuckGo Instant Answer API returns.
type wireResult struct {
	Heading        string `json:"Heading"`
	AbstractText   string `json:"AbstractText"`
	AbstractSource string `json:"AbstractSource"`
	AbstractURL    string `json:"AbstractURL"`
	Type           string `json:"Type"`
	Entity         string `json:"Entity"`
	RelatedTopics  []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
	} `json:"RelatedTopics"`
}

// Answer is the structured instant answer for a query.
type Answer struct {
	Heading  string `kit:"id" json:"heading"`
	Abstract string `json:"abstract"`
	Source   string `json:"source"`
	URL      string `json:"url"`
	Type     string `json:"type"`
	Entity   string `json:"entity"`
	Topics   int    `json:"related_topics"`
}

// Search fetches the instant answer for the given query from the DuckDuckGo
// Instant Answer API and returns a single Answer record.
// skipDisambig=true adds skip_disambig=1 to the request to skip disambiguation
// pages and return the first direct match.
func (c *Client) Search(ctx context.Context, query string, skipDisambig bool) (*Answer, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("no_redirect", "1")
	params.Set("no_html", "1")
	if skipDisambig {
		params.Set("skip_disambig", "1")
	}
	rawURL := BaseURL + "/?" + params.Encode()

	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	var wire wireResult
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if wire.Heading == "" && wire.AbstractText == "" {
		return nil, fmt.Errorf("no instant answer found for %q", query)
	}

	return &Answer{
		Heading:  wire.Heading,
		Abstract: wire.AbstractText,
		Source:   wire.AbstractSource,
		URL:      wire.AbstractURL,
		Type:     wire.Type,
		Entity:   wire.Entity,
		Topics:   len(wire.RelatedTopics),
	}, nil
}

// Get fetches rawURL and returns the response body. It paces and retries
// according to the client's settings. The caller owns nothing extra; the body
// is read fully and closed here.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
