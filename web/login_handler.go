package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/types"
)

func (h *Handler) showLogin(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "login.tmpl", nil, http.StatusOK)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseForm(); err != nil {
		h.renderWithError(w, r, "login.tmpl", nil, fmt.Errorf("parse form: %w", err))
		return
	}

	ctx := r.Context()
	in := types.UpsertUser{
		Email:    r.PostFormValue("email"),
		Username: strings.SplitN(r.PostFormValue("email"), "@", 2)[0],
	}
	out, err := h.Service.UpsertUser(ctx, in)
	if err != nil {
		h.renderWithError(w, r, "login.tmpl", map[string]any{
			"Form": in,
		}, fmt.Errorf("upsert user: %w", err))
		return
	}

	h.sess.Put(ctx, "logged_in_user_id", out.ID)

	if err := h.sess.RenewToken(ctx); err != nil {
		h.renderWithError(w, r, "login.tmpl", map[string]any{
			"Form": in,
		}, fmt.Errorf("renew session token: %w", err))
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if !h.sess.Exists(ctx, "logged_in_user_id") {
			next.ServeHTTP(w, r)
			return
		}

		loggedInUserID := h.sess.GetString(ctx, "logged_in_user_id")
		user, err := h.Service.User(ctx, types.RetrieveUser{
			UserID: loggedInUserID,
		})
		if err != nil {
			h.renderErrorPage(w, r, fmt.Errorf("get user: %w", err))
			return
		}

		ctx = auth.ContextWithUser(ctx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.sess.Remove(ctx, "logged_in_user_id")
	if err := h.sess.RenewToken(ctx); err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("renew session token: %w", err))
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
