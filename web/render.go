package web

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/validator"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"mvdan.cc/xurls/v2"
)

var errPageNotFound = errs.NewNotFoundError("page not found")

func (h *Handler) render(w http.ResponseWriter, r *http.Request, name string, data map[string]any, statusCode int) {
	if data == nil {
		data = map[string]any{}
	}

	ctx := r.Context()
	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	data["LoggedInUser"] = loggedInUser
	data["LoggedIn"] = loggedIn
	data["MinioURL"] = h.MinioURL
	data["Request"] = r

	if h.sess.Exists(ctx, "error") {
		errorVal := h.sess.Pop(ctx, "error")

		switch v := errorVal.(type) {
		case *validator.Validator, *errs.Error:
			data["Error"] = v
		case string:
			data["Error"] = errors.New(v)
		default:
			data["Error"] = v
		}
	}

	var buff bytes.Buffer
	if err := h.renderer.Render(&buff, name, data); err != nil {
		h.ErrorLogger.Error("render template", "template", name, "err", err)
		if name == "error.tmpl" {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		} else {
			h.renderErrorPage(w, r, fmt.Errorf("render template %s: %w", name, err))
		}
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	if _, err := buff.WriteTo(w); err != nil && !errors.Is(err, syscall.EPIPE) {
		h.ErrorLogger.Error("write response", "req_method", r.Method, "req_url", r.URL.String(), "err", err)
		h.renderErrorPage(w, r, fmt.Errorf("write response: %w", err))
		return
	}
}

var allowedNotFound = map[string]bool{
	"/favicon.ico":                      true,
	"/apple-touch-icon-precomposed.png": true,
	"/apple-touch-icon.png":             true,
}

func isAllowedNotFound(path string) bool {
	return allowedNotFound[path]
}

func (h *Handler) renderErrorPage(w http.ResponseWriter, r *http.Request, err error) {
	if !isAllowedNotFound(r.URL.Path) && !errors.Is(err, errPageNotFound) {
		h.ErrorLogger.Error("got error", "req_method", r.Method, "req_url", r.URL.String(), "err", err)
	}
	h.render(w, r, "error.tmpl", map[string]any{
		"Error": maskError(err),
	}, http.StatusInternalServerError)
}

func (h *Handler) renderWithError(w http.ResponseWriter, r *http.Request, name string, data map[string]any, err error) {
	if data == nil {
		data = map[string]any{}
	}
	h.ErrorLogger.Error("got error", "req_method", r.Method, "req_url", r.URL.String(), "err", err)
	data["Error"] = maskError(err)
	h.render(w, r, name, data, errorToStatusCode(err))
}

func maskError(err error) error {
	var errValidator *validator.Validator
	var errTypes *errs.Error

	if errors.As(err, &errValidator) || errors.As(err, &errTypes) {
		return err
	}

	return fmt.Errorf("an unexpected error occurred: %w", err)
}

func errorToStatusCode(err error) int {
	var errValidator *validator.Validator
	if errors.As(err, &errValidator) {
		return http.StatusUnprocessableEntity
	}

	var errTypes *errs.Error
	if errors.As(err, &errTypes) {
		switch errTypes.Kind {
		case errs.KindNotFound:
			return http.StatusNotFound
		case errs.KindInvalidArgument:
			return http.StatusUnprocessableEntity
		case errs.KindUnauthenticated:
			return http.StatusUnauthorized
		case errs.KindPermissionDenied:
			return http.StatusForbidden
		case errs.KindAlreadyExists:
			return http.StatusConflict
		default:
			return http.StatusInternalServerError
		}
	}

	return http.StatusInternalServerError
}

func init() {
	gob.Register(&validator.Validator{})
	gob.Register(&errs.Error{})
}

func (h *Handler) redirectBackWithError(w http.ResponseWriter, r *http.Request, err error) {
	var errValidator *validator.Validator
	var errTypes *errs.Error

	h.ErrorLogger.Error("got error", "req_method", r.Method, "req_url", r.URL.String(), "err", err)

	if errors.As(err, &errValidator) {
		h.sess.Put(r.Context(), "error", errValidator)
	} else if errors.As(err, &errTypes) {
		h.sess.Put(r.Context(), "error", errTypes)
	} else {
		h.sess.Put(r.Context(), "error", maskError(err).Error())
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

var funcMap = template.FuncMap{
	"strings": func() stringsModule {
		return stringsModule{}
	},
	"url": func() urlModule {
		return urlModule{}
	},
	"plus":    plus,
	"linkify": linkify,
}

type stringsModule struct{}

func (s stringsModule) Title(str string) string {
	return cases.Title(language.English).String(str)
}

type urlModule struct{}

func (m urlModule) WithQuery(baseURL *url.URL, dict map[string]any) template.URL {
	u := cloneURL(baseURL)
	q := u.Query()
	for key, value := range dict {
		q.Set(key, stringify(value))
	}
	if _, ok := dict["before"]; ok && q.Has("after") {
		q.Del("after")
	}
	if _, ok := dict["after"]; ok && q.Has("before") {
		q.Del("before")
	}
	u.RawQuery = q.Encode()
	return template.URL(u.String())
}

func stringify(x any) string {
	val := reflect.ValueOf(x)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return "null"
		}

		x = val.Elem().Interface()
	}

	switch v := x.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", x)
	}
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	u2 := new(url.URL)
	*u2 = *u
	if u.User != nil {
		u2.User = new(url.Userinfo)
		*u2.User = *u.User
	}
	return u2
}

func plus(args ...any) (any, error) {
	if len(args) == 0 {
		return nil, nil
	}

	if len(args) == 1 {
		return args[0], nil
	}

	// Convert all arguments to float64 for calculation
	var sum float64
	var hasFloat bool

	for i, arg := range args {
		switch v := arg.(type) {
		case int:
			sum += float64(v)
		case int8:
			sum += float64(v)
		case int16:
			sum += float64(v)
		case int32:
			sum += float64(v)
		case int64:
			sum += float64(v)
		case uint:
			sum += float64(v)
		case uint8:
			sum += float64(v)
		case uint16:
			sum += float64(v)
		case uint32:
			sum += float64(v)
		case uint64:
			sum += float64(v)
		case float32:
			sum += float64(v)
			hasFloat = true
		case float64:
			sum += v
			hasFloat = true
		default:
			return nil, fmt.Errorf("argument %d: unsupported type %T (expected numeric type)", i, arg)
		}
	}

	// Return the appropriate type based on inputs
	if hasFloat {
		return sum, nil
	}

	// If all inputs were integers, try to return an integer
	// Check if the result fits in int64
	if sum >= float64(^uint64(0)>>1) || sum < float64(-1<<63) {
		return sum, nil // Return float64 if too large for int64
	}

	return int64(sum), nil
}

var (
	reURL      = xurls.Strict()
	reMentions = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_-])@([a-zA-Z0-9][a-zA-Z0-9_\.-]*)`)
)

// linkify converts URLs and @mentions in text to clickable HTML links
func linkify(text string) template.HTML {
	if text == "" {
		return template.HTML("")
	}

	// SECURITY: First escape any HTML in the input text to prevent XSS
	escapedText := template.HTMLEscapeString(text)

	// First convert URLs to links using the existing reURL regex
	result := reURL.ReplaceAllStringFunc(escapedText, func(url string) string {
		return fmt.Sprintf(`<a href="%[1]s" target="_blank" rel="noreferrer noopener" class="primary">%[1]s</a>`, url)
	})

	// Then convert @mentions to links using the existing reMentions regex
	result = reMentions.ReplaceAllStringFunc(result, func(match string) string {
		// Extract the username from the match
		// The username is everything after the @ symbol
		atIndex := strings.LastIndex(match, "@")
		if atIndex == -1 {
			return match
		}

		rawUsername := match[atIndex+1:]

		// Clean and validate the username
		cleanedUsername := cleanMentionUsername(rawUsername)
		if cleanedUsername == "" || utf8.RuneCountInString(cleanedUsername) > 21 {
			return match
		}

		// Check if this mention is part of an email
		if isPartOfMentionEmail(escapedText, match) {
			return match
		}

		// Get the context character(s) before @
		contextChar := match[:atIndex]

		// Find any trailing punctuation that was removed during cleaning
		trailingPunctuation := ""
		if len(rawUsername) > len(cleanedUsername) {
			trailingPunctuation = rawUsername[len(cleanedUsername):]
		}

		// Generate the link
		link := fmt.Sprintf(`<a href="/u/%s" class="primary">@%s</a>`, cleanedUsername, cleanedUsername)

		return contextChar + link + trailingPunctuation
	})

	return template.HTML(result)
}

// cleanMentionUsername removes trailing punctuation that's not part of the username
func cleanMentionUsername(username string) string {
	// Remove trailing dots that are likely sentence punctuation
	for len(username) > 0 && username[len(username)-1] == '.' {
		username = username[:len(username)-1]
	}
	return username
}

// isPartOfMentionEmail checks if the @username is part of an email address
func isPartOfMentionEmail(escapedText, fullMatch string) bool {
	// Find the position of the full match in the escaped text
	pos := strings.Index(escapedText, fullMatch)
	if pos == -1 {
		return false
	}

	// Check if there's another @ character immediately after the username
	endPos := pos + len(fullMatch)
	if endPos < len(escapedText) && escapedText[endPos] == '@' {
		return true
	}

	return false
}

func (h *Handler) buildMinioURL(bucket, path string) string {
	if bucket == "" || path == "" {
		return ""
	}
	base := strings.TrimSuffix(h.MinioURL, "/")
	bucket = strings.TrimPrefix(strings.TrimSuffix(bucket, "/"), "/")
	path = strings.TrimPrefix(path, "/")

	return fmt.Sprintf("%s/%s/%s", base, bucket, path)
}
