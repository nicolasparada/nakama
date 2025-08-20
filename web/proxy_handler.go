package web

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
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

			// Remove the Cookie, Referer, and Authorization headers
			newreq.Header.Del("Cookie")
			newreq.Header.Del("Referer")
			newreq.Header.Del("Authorization")

			// Forward cache headers for conditional requests
			if inm := r.Header.Get("If-None-Match"); inm != "" {
				newreq.Header.Set("If-None-Match", inm)
			}
			if ims := r.Header.Get("If-Modified-Since"); ims != "" {
				newreq.Header.Set("If-Modified-Since", ims)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// Remove the WWW-Authenticate header to prevent the browser from showing the basic auth pop-up dialog.
			if resp.Header.Get("Www-Authenticate") != "" {
				resp.Header.Del("Www-Authenticate")
			}

			ct := resp.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "image/") {
				resp.Body.Close()
				resp.StatusCode = http.StatusForbidden
				resp.Header = make(http.Header)
				resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
				resp.Body = http.NoBody
			}

			return nil
		},
	}
	proxy.ServeHTTP(w, r)
}
