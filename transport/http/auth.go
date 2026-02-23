package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/types"
)

type loginInput struct {
	Email string
}

func (h *handler) sendMagicLink(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in types.SendMagicLink
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	err := h.svc.SendMagicLink(r.Context(), in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) verifyMagicLink(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirectURI, err := h.svc.ParseRedirectURI(q.Get("redirect_uri"))
	if err != nil {
		h.respondErr(w, err)
		return
	}

	ctx := r.Context()
	in := types.UseEmailVerificationCode{
		Email:    q.Get("email"),
		Code:     q.Get("code"),
		Username: emptyStrPtr(q.Get("username")),
	}
	auth, err := h.svc.VerifyMagicLink(ctx, in)
	if err == service.ErrUserNotFound || err == service.ErrInvalidUsername || err == service.ErrUsernameTaken {
		redirectWithHashFragment(w, r, redirectURI, url.Values{
			"error":          []string{err.Error()},
			"retry_endpoint": []string{r.RequestURI},
		}, http.StatusFound)
		return
	}

	if err != nil {
		statusCode := err2code(err)
		if statusCode != http.StatusInternalServerError {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{err.Error()},
			}, http.StatusFound)
			return
		}

		if !errors.Is(err, context.Canceled) {
			_ = h.logger.Log("err", err)
		}
		redirectWithHashFragment(w, r, redirectURI, url.Values{
			"error": []string{"internal server error"},
		}, http.StatusFound)
		return
	}

	values := url.Values{
		"token":         []string{auth.Token},
		"expires_at":    []string{auth.ExpiresAt.Format(time.RFC3339Nano)},
		"user.id":       []string{auth.User.ID},
		"user.username": []string{auth.User.Username},
	}
	if auth.User.AvatarURL != nil {
		values.Set("user.avatar_url", *auth.User.AvatarURL)
	}
	redirectWithHashFragment(w, r, redirectURI, values, http.StatusFound)
}

func (h *handler) devLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in loginInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	out, err := h.svc.DevLogin(r.Context(), in.Email)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, out, http.StatusOK)
}

func (h *handler) authUser(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.AuthUser(r.Context())
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, u, http.StatusOK)
}

func (h *handler) token(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Token(r.Context())
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, out, http.StatusOK)
}

func (h *handler) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(r.URL.Query().Get("auth_token"))

		if token == "" {
			if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
				token = a[7:]
			}
		}

		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		uid, err := h.svc.AuthUserIDFromToken(token)
		if err != nil {
			h.respondErr(w, err)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, service.KeyAuthUserID, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
