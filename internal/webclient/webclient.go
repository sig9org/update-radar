// Package webclient fetches a URL and reduces its body to a short digest,
// so web-rader can tell whether a page's content changed between checks.
package webclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

// userAgent identifies web-rader to the sites it checks.
const userAgent = "web-rader/1.0 (+https://github.com/sig9org/update-radar)"

// DefaultTimeout bounds a single site fetch.
const DefaultTimeout = 30 * time.Second

// Client fetches site content over HTTP.
type Client struct {
	HTTPClient *http.Client
}

// NewClient builds a Client with the supplied timeout. A non-positive timeout
// uses DefaultTimeout.
func NewClient(timeout ...time.Duration) *Client {
	t := DefaultTimeout
	if len(timeout) > 0 && timeout[0] > 0 {
		t = timeout[0]
	}
	return &Client{HTTPClient: &http.Client{Timeout: t}}
}

// Fetch retrieves url and returns the SHA-256 hex digest of its response
// body, so callers can detect content changes without keeping the full page
// around between checks.
func (c *Client) Fetch(ctx context.Context, url string) (digest string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("webclient: build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("webclient: request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("webclient: get %s: unexpected status %d", url, resp.StatusCode)
	}

	h := sha256.New()
	if _, err := io.Copy(h, resp.Body); err != nil {
		return "", fmt.Errorf("webclient: read response for %s: %w", url, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
