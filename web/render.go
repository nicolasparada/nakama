package web

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"html/template"
	"net/http"
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

func (h *Handler) render(w http.ResponseWriter, r *http.Request, name string, data map[string]any, statusCode int) {
	if data == nil {
		data = map[string]any{}
	}

	ctx := r.Context()
	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	data["LoggedInUser"] = loggedInUser
	data["LoggedIn"] = loggedIn

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

func (h *Handler) renderErrorPage(w http.ResponseWriter, r *http.Request, err error) {
	h.ErrorLogger.Error("got error", "req_method", r.Method, "req_url", r.URL.String(), "err", err)
	h.render(w, r, "error.tmpl", map[string]any{
		"Error": maskError(err),
	}, http.StatusInternalServerError)
}

func (h *Handler) renderWithError(w http.ResponseWriter, r *http.Request, name string, data map[string]any, err error) {
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
	"plus":    plus,
	"linkify": linkify,
}

type stringsModule struct{}

func (s stringsModule) Title(str string) string {
	return cases.Title(language.English).String(str)
}

func plus(args ...any) (any, error) {
	if len(args) == 0 {
		return nil, nil
	}

	if len(args) == 1 {
		return args[0], nil
	}

	switch v := args[0].(type) {
	case int:
		for _, arg := range args[1:] {
			n, ok := arg.(int)
			if !ok {
				return nil, fmt.Errorf("expected int argument, got %T", arg)
			}
			v += n
		}
		return v, nil
	case int32:
		for _, arg := range args[1:] {
			n, ok := arg.(int32)
			if !ok {
				return nil, fmt.Errorf("expected int32 argument, got %T", arg)
			}
			v += n
		}
		return v, nil
	case int64:
		for _, arg := range args[1:] {
			n, ok := arg.(int64)
			if !ok {
				return nil, fmt.Errorf("expected int64 argument, got %T", arg)
			}
			v += n
		}
		return v, nil
	case uint32:
		for _, arg := range args[1:] {
			n, ok := arg.(uint32)
			if !ok {
				return nil, fmt.Errorf("expected uint32 argument, got %T", arg)
			}
			v += n
		}
		return v, nil
	case uint64:
		for _, arg := range args[1:] {
			n, ok := arg.(uint64)
			if !ok {
				return nil, fmt.Errorf("expected uint64 argument, got %T", arg)
			}
			v += n
		}
		return v, nil
	case float64:
		for _, arg := range args[1:] {
			n, ok := arg.(float64)
			if !ok {
				return nil, fmt.Errorf("expected float64 argument, got %T", arg)
			}
			v += n
		}
		return v, nil
	}

	return nil, fmt.Errorf("unsupported argument type")
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

	// First convert URLs to links using the existing reURL regex
	result := reURL.ReplaceAllStringFunc(text, func(url string) string {
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
		if isPartOfMentionEmail(text, match) {
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
func isPartOfMentionEmail(text, fullMatch string) bool {
	// Find the position of the full match in the text
	pos := strings.Index(text, fullMatch)
	if pos == -1 {
		return false
	}

	// Check if there's another @ character immediately after the username
	endPos := pos + len(fullMatch)
	if endPos < len(text) && text[endPos] == '@' {
		return true
	}

	return false
}
