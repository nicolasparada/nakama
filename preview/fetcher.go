package preview

import (
	"context"
	"errors"
	"fmt"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gen2brain/avif"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/nicolasparada/nakama/opengraph"
	"golang.org/x/image/webp"
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

	if _, found := f.cacheErr.Get(urlStr); found {
		return empty, ErrBackoff
	}

	if og, found := f.cacheOK.Get(urlStr); found {
		return og, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		f.cacheErr.Add(urlStr, struct{}{})
		return empty, fmt.Errorf("new request: %w", err)
	}

	// Use a bot-friendly UA that still works with most sites including X.com
	req.Header.Set("User-Agent", "Twitterbot/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/*;q=0.8,*/*;q=0.5")

	resp, err := f.client.Do(req)
	if err != nil {
		f.cacheErr.Add(urlStr, struct{}{})
		return empty, fmt.Errorf("http get: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		f.cacheErr.Add(urlStr, struct{}{})

		if b, err := io.ReadAll(resp.Body); err == nil {
			return empty, fmt.Errorf("http status: %d, body: %s", resp.StatusCode, string(b))
		}

		return empty, fmt.Errorf("http status: %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, f.maxBytes)

	contentType := resp.Header.Get("Content-Type")
	if ct, _, err := mime.ParseMediaType(contentType); err == nil && strings.HasPrefix(ct, "image/") {
		og := opengraph.OpenGraph{
			Title:    extractImageNameFromURL(urlStr),
			URL:      urlStr,
			Type:     "image",
			SiteName: extractHostFromURL(urlStr),
			Images: []opengraph.Image{{
				URL:  urlStr,
				Type: ct,
			}},
		}
		if w, h, err := getImageResolution(limited, ct); err == nil {
			og.Images[0].Width = w
			og.Images[0].Height = h
		}
		f.cacheOK.Add(urlStr, og)
		return og, nil
	}

	og, parseErr := opengraph.Parse(limited, urlStr)
	if parseErr != nil {
		f.cacheErr.Add(urlStr, struct{}{})
		return empty, fmt.Errorf("parse opengraph: %w", parseErr)
	}

	f.cacheOK.Add(urlStr, og)
	return og, nil
}

func extractImageNameFromURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	return filepath.Base(u.Path)
}

func extractHostFromURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func getImageResolution(r io.Reader, contentType string) (uint32, uint32, error) {
	switch contentType {
	case "image/jpeg":
		img, err := jpeg.Decode(r)
		if err != nil {
			return 0, 0, fmt.Errorf("decode jpeg: %w", err)
		}
		return uint32(img.Bounds().Dx()), uint32(img.Bounds().Dy()), nil
	case "image/png":
		img, err := png.Decode(r)
		if err != nil {
			return 0, 0, fmt.Errorf("decode png: %w", err)
		}
		return uint32(img.Bounds().Dx()), uint32(img.Bounds().Dy()), nil
	case "image/gif":
		img, err := gif.Decode(r)
		if err != nil {
			return 0, 0, fmt.Errorf("decode gif: %w", err)
		}
		return uint32(img.Bounds().Dx()), uint32(img.Bounds().Dy()), nil
	case "image/webp":
		img, err := webp.Decode(r)
		if err != nil {
			return 0, 0, fmt.Errorf("decode webp: %w", err)
		}
		return uint32(img.Bounds().Dx()), uint32(img.Bounds().Dy()), nil
	case "image/avif":
		img, err := avif.Decode(r)
		if err != nil {
			return 0, 0, fmt.Errorf("decode avif: %w", err)
		}
		return uint32(img.Bounds().Dx()), uint32(img.Bounds().Dy()), nil
	default:
		return 0, 0, fmt.Errorf("unsupported image type: %s", contentType)
	}
}
