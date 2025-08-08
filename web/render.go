package web

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"syscall"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/validator"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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
}

type stringsModule struct{}

func (s stringsModule) Title(str string) string {
	return cases.Title(language.English).String(str)
}
