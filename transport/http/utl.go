package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
	"github.com/nicolasparada/go-errs/httperrs"
)

const proxyCacheControl = time.Hour * 24 * 14

const errInvalidTargetURL = errs.InvalidArgumentError("invalid target URL")

var (
	errBadRequest           = errors.New("bad request")
	errStreamingUnsupported = errors.New("streaming unsupported")
	errTeaPot               = errors.New("i am a teapot")
	errOauthTimeout         = errors.New("oauth timeout")
	errEmailNotVerified     = errors.New("email not verified")
	errEmailNotProvided     = errors.New("email not provided")
	errServiceUnavailable   = errors.New("service unavailable")
)

func (h *handler) respond(w http.ResponseWriter, v any, statusCode int) {
	b, err := json.Marshal(v)
	if err != nil {
		h.respondErr(w, fmt.Errorf("could not json marshal http response body: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_, err = w.Write(b)
	if err != nil && !errors.Is(err, syscall.EPIPE) && !errors.Is(err, context.Canceled) {
		_ = h.logger.Log("err", fmt.Errorf("could not write down http response: %w", err))
	}
}

func (h *handler) respondErr(w http.ResponseWriter, err error) {
	statusCode := err2code(err)
	if statusCode == http.StatusInternalServerError {
		if !errors.Is(err, context.Canceled) {
			_ = h.logger.Log("err", err)
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Error(w, err.Error(), statusCode)
}

func err2code(err error) int {
	if err == nil {
		return http.StatusOK
	}
	switch {
	case errors.Is(err, errBadRequest) ||
		errors.Is(err, errOauthTimeout) ||
		errors.Is(err, errEmailNotVerified) ||
		errors.Is(err, errEmailNotProvided):
		return http.StatusBadRequest
	case errors.Is(err, errStreamingUnsupported):
		return http.StatusExpectationFailed
	case errors.Is(err, errTeaPot):
		return http.StatusTeapot
	case errors.Is(err, errServiceUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, service.ErrUnimplemented):
		return http.StatusNotImplemented
	}

	return httperrs.Code(err)
}

func (h *handler) writeSSE(w io.Writer, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		_ = h.logger.Log("err", fmt.Errorf("could not json marshal sse data: %w", err))
		_, errWrite := fmt.Fprintf(w, "event: error\ndata: %v\n\n", err)
		if errWrite != nil && !errors.Is(errWrite, syscall.EPIPE) {
			_ = h.logger.Log("err", fmt.Errorf("could not write sse error: %w", errWrite))
		}
		return
	}

	_, errWrite := fmt.Fprintf(w, "data: %s\n\n", b)
	if errWrite != nil && !errors.Is(errWrite, syscall.EPIPE) {
		_ = h.logger.Log("err", fmt.Errorf("could not write sse data: %w", errWrite))
	}
}

func (h *handler) proxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	targetStr := r.URL.Query().Get("target")
	if targetStr == "" {
		h.respondErr(w, errInvalidTargetURL)
		return
	}

	target, err := url.Parse(targetStr)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil {
		h.respondErr(w, errInvalidTargetURL)
		return
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = pr.Out.URL.Host
			pr.Out.Header.Del("Cookie")              // Drop browser cookies.
			pr.Out.Header.Del("Authorization")       // Drop app credentials.
			pr.Out.Header.Del("Proxy-Authorization") // Drop proxy credentials.
			pr.Out.Header.Del("Origin")              // Drop caller origin.
			pr.Out.Header.Del("Referer")             // Drop caller page.
			pr.Out.Header.Del("X-Real-Ip")           // Drop client IP hints.
			pr.Out.Header.Del("Forwarded")           // Drop forwarded chain.
			pr.Out.Header.Del("X-Forwarded-For")     // Drop forwarded client IPs.
			pr.Out.Header.Del("X-Forwarded-Host")    // Drop forwarded host.
			pr.Out.Header.Del("X-Forwarded-Proto")   // Drop forwarded scheme.

			// Keep Go from adding its default User-Agent upstream.
			if _, ok := pr.Out.Header["User-Agent"]; !ok {
				pr.Out.Header.Set("User-Agent", "")
			}
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.WriteHeader(http.StatusBadGateway)
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")         // Drop upstream cookies.
			resp.Header.Del("Set-Cookie2")        // Drop legacy cookies.
			resp.Header.Del("Clear-Site-Data")    // Drop site-wide clears.
			resp.Header.Del("Www-Authenticate")   // Drop browser auth prompts.
			resp.Header.Del("Proxy-Authenticate") // Drop proxy auth prompts.
			return nil
		},
	}
	proxy.ServeHTTP(w, r)
}

func withCacheControl(d time.Duration) func(http.HandlerFunc) http.HandlerFunc {
	return func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d, public", int64(d.Seconds())))
			h(w, r)
		}
	}
}

func redirectWithHashFragment(w http.ResponseWriter, r *http.Request, uri *url.URL, data url.Values, statusCode int) {
	uri.Fragment = data.Encode()
	http.Redirect(w, r, uri.String(), statusCode)
}

func parsePageArgs(q url.Values) (types.PageArgs, error) {
	var pageArgs types.PageArgs

	if q.Has("first") {
		first, err := strconv.ParseUint(q.Get("first"), 10, 64)
		if err != nil {
			return pageArgs, errs.InvalidArgumentError("invalid first page arg")
		}

		pageArgs.First = new(uint(first))
	}

	if q.Has("after") {
		pageArgs.After = new(q.Get("after"))
	}

	if q.Has("last") {
		last, err := strconv.ParseUint(q.Get("last"), 10, 64)
		if err != nil {
			return pageArgs, errs.InvalidArgumentError("invalid last page arg")
		}

		pageArgs.Last = new(uint(last))
	}

	if q.Has("before") {
		pageArgs.Before = new(q.Get("before"))
	}

	return pageArgs, nil
}
