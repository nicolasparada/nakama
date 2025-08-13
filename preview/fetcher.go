package preview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/nicolasparada/nakama/opengraph"
)

// ErrBackoff is returned when a previous fetch/parse failed recently and retries are being throttled.
var ErrBackoff = errors.New("preview fetch backoff due to recent error")

// Fetcher fetches and caches OpenGraph previews for URLs.
type Fetcher struct {
	client     *http.Client
	cacheOK    *lru.LRU[string, opengraph.OpenGraph]
	cacheErr   *lru.LRU[string, struct{}]
	successTTL time.Duration
	errorTTL   time.Duration
	maxBytes   int64
}

// NewFetcher creates a new Fetcher.
// size controls the LRU size for both success and error caches.
// successTTL is how long successful previews are cached.
// errorTTL is how long failures are cached to avoid repeated attempts.
// If client is nil, a default client with a 10s timeout is used.
func NewFetcher(size int, successTTL, errorTTL time.Duration, client *http.Client) *Fetcher {
	if size <= 0 {
		size = 256
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	cacheOK := lru.NewLRU[string, opengraph.OpenGraph](size, nil, successTTL)
	cacheErr := lru.NewLRU[string, struct{}](size, nil, errorTTL)
	return &Fetcher{
		client:     client,
		cacheOK:    cacheOK,
		cacheErr:   cacheErr,
		successTTL: successTTL,
		errorTTL:   errorTTL,
		maxBytes:   2 << 20, // 2 MiB cap for HTML
	}
}

// Get retrieves the OpenGraph preview for urlStr, consulting caches first.
// It negative-caches on fetch or parse error to prevent repeated attempts for errorTTL.
func (f *Fetcher) Get(ctx context.Context, urlStr string) (opengraph.OpenGraph, error) {
	var empty opengraph.OpenGraph
	key := canonicalURL(urlStr)

	if _, found := f.cacheErr.Get(key); found {
		return empty, ErrBackoff
	}

	if og, found := f.cacheOK.Get(key); found {
		return og, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, key, nil)
	if err != nil {
		f.cacheErr.Add(key, struct{}{})
		return empty, fmt.Errorf("new request: %w", err)
	}

	// Use a bot-friendly UA that still works with most sites including X.com
	req.Header.Set("User-Agent", "Twitterbot/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		f.cacheErr.Add(key, struct{}{})
		return empty, fmt.Errorf("http get: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		f.cacheErr.Add(key, struct{}{})

		if b, err := io.ReadAll(resp.Body); err == nil {
			return empty, fmt.Errorf("http status: %d, body: %s", resp.StatusCode, string(b))
		}

		return empty, fmt.Errorf("http status: %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, f.maxBytes)
	og, parseErr := opengraph.Parse(limited)
	if parseErr != nil {
		f.cacheErr.Add(key, struct{}{})
		return empty, fmt.Errorf("parse opengraph: %w", parseErr)
	}

	f.cacheOK.Add(key, og)
	return og, nil
}

func canonicalURL(u string) string {
	s := strings.TrimSpace(u)
	if s == "" {
		return s
	}

	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}

	return "https://" + s
}
