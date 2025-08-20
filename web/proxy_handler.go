package web

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

func (h *Handler) proxy(w http.ResponseWriter, r *http.Request) {
	// TODO: prevent usage of the proxy from 3rd party sites.
	// check on origin/referer

	target := r.URL.Query().Get("url")
	if target == "" {
		http.Error(w, "Missing URL", http.StatusUnprocessableEntity)
		return
	}
	u, err := url.Parse(target)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid URL: %v", err), http.StatusUnprocessableEntity)
		return
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		http.Error(w, "Invalid URL scheme", http.StatusUnprocessableEntity)
		return
	}

	proxy := &httputil.ReverseProxy{
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.WriteHeader(http.StatusBadGateway)
		},
		Director: func(newreq *http.Request) {
			newreq.URL = u
			newreq.Host = u.Host

			// taken from httputil.NewSingleHostReverseProxy
			if _, ok := newreq.Header["User-Agent"]; !ok {
				// explicitly disable User-Agent so it's not set to default value
				newreq.Header.Set("User-Agent", "")
			}

			newreq.Header.Del("Cookie")
			newreq.Header.Del("Authorization")
		},
		ModifyResponse: func(resp *http.Response) error {
			ct := resp.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "image/") {
				resp.Body.Close()
				resp.StatusCode = http.StatusForbidden
				resp.Header = make(http.Header)
				resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
				resp.Body = http.NoBody
			}

			// Remove the WWW-Authenticate header to prevent the browser from showing the basic auth pop-up dialog.
			if resp.Header.Get("Www-Authenticate") != "" {
				resp.Header.Del("Www-Authenticate")
			}

			// Set caching headers
			dur := time.Hour * 24
			resp.Header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(dur.Seconds())))
			resp.Header.Set("Pragma", "public")
			resp.Header.Set("Expires", fmt.Sprintf("%d", time.Now().Add(dur).Unix()))

			// Prevent setting cookies in the user's browser.
			resp.Header.Del("Set-Cookie")

			return nil
		},
	}
	proxy.ServeHTTP(w, r)
}
