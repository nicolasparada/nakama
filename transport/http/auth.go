package http

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

const (
	sessKeyOauthRedirectURI = "sess_oauth_redirect_uri"
	sessKeyOauthState       = "sess_oauth_state"
	sessKeyPendingSignup    = "sess_pending_signup"
	sessKeyUserID           = "sess_user_id"
)

func init() {
	// scs uses gob to encode session data, so we need to register our custom types.
	gob.Register(&url.URL{})            // for sessKeyOauthRedirectURI
	gob.Register(types.PendingSignup{}) // for sessKeyPendingSignup
}

func (h *handler) requestLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in types.RequestLogin
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ctx := r.Context()
	err := h.svc.RequestLogin(ctx, in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) verifyLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ctx := r.Context()
	resp, err := h.svc.VerifyLogin(ctx, in.Code)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	switch resp.Status {
	case types.LoginResultSuccess:
		h.sess.Put(ctx, sessKeyUserID, resp.User.ID)
		if err := h.sess.RenewToken(ctx); err != nil {
			h.respondErr(w, err)
			return
		}

	case types.LoginResultPendingSignup:
		h.sess.Put(ctx, sessKeyPendingSignup, resp.PendingSignup)
	}

	h.respond(w, resp, http.StatusOK)
}

func (h *handler) oauthRedirect(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")

	state, err := genOAuthState()
	if err != nil {
		h.respondErr(w, fmt.Errorf("generate oauth state: %w", err))
		return
	}

	redirectURI, err := url.Parse(r.URL.Query().Get("redirect_uri"))
	if err != nil || !redirectURI.IsAbs() {
		h.respondErr(w, errs.InvalidArgumentError("invalid redirect uri"))
		return
	}

	ctx := r.Context()

	oauthURL, err := h.svc.OAuthURL(ctx, providerName, state)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.sess.Put(ctx, sessKeyOauthState, state)
	h.sess.Put(ctx, sessKeyOauthRedirectURI, redirectURI)
	http.Redirect(w, r, oauthURL, http.StatusFound)
}

func (h *handler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	redirectURI, ok := h.sess.Pop(ctx, sessKeyOauthRedirectURI).(*url.URL)
	if !ok {
		h.respondErr(w, errs.InvalidArgumentError("redirect URI not found in session"))
		return
	}

	state := h.sess.PopString(ctx, sessKeyOauthState)
	if state == "" {
		h.logger.Log("msg", "oauth state not found in session")
		redirectWithHashFragment(w, r, redirectURI, url.Values{
			"result": []string{"error"},
			"error":  []string{"oauth state not found in session"},
		}, http.StatusFound)
		return
	}

	q := r.URL.Query()

	if q.Get("state") != state {
		h.logger.Log("msg", "invalid oauth state", "expected", state, "got", q.Get("state"))
		redirectWithHashFragment(w, r, redirectURI, url.Values{
			"result": []string{"error"},
			"error":  []string{"invalid oauth state"},
		}, http.StatusFound)
		return
	}

	code := q.Get("code")
	providerName := r.PathValue("provider")

	resp, err := h.svc.OAuthLogin(ctx, providerName, code)
	if err != nil {
		h.logger.Log("msg", "oauth login failed", "err", err)
		redirectWithHashFragment(w, r, redirectURI, url.Values{
			"result": []string{"error"},
			"error":  []string{err.Error()},
		}, http.StatusFound)
		return
	}

	switch resp.Status {
	case types.LoginResultSuccess:
		h.sess.Put(ctx, sessKeyUserID, resp.User.ID)
		if err := h.sess.RenewToken(ctx); err != nil {
			h.logger.Log("msg", "renew session token", "err", err)
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"result": []string{"error"},
				"error":  []string{fmt.Sprintf("renew session token: %v", err)},
			}, http.StatusFound)
			return
		}

		redirectWithHashFragment(w, r, redirectURI, url.Values{
			"result": []string{"success"},
		}, http.StatusFound)

	case types.LoginResultPendingSignup:
		h.sess.Put(ctx, sessKeyPendingSignup, resp.PendingSignup)
		redirectWithHashFragment(w, r, redirectURI, url.Values{
			"result": []string{"pending_signup"},
		}, http.StatusFound)
	}
}

func (h *handler) completeSignup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pendingSignup, ok := h.sess.Get(ctx, sessKeyPendingSignup).(types.PendingSignup)
	if !ok {
		h.respondErr(w, errs.InvalidArgumentError("no pending signup"))
		return
	}

	if pendingSignup.IsExpired() {
		h.sess.Remove(ctx, sessKeyPendingSignup)
		h.respondErr(w, errs.InvalidArgumentError("pending signup has expired"))
		return
	}

	defer r.Body.Close()

	var in struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errs.InvalidArgumentError("invalid request body"))
		return
	}

	pendingSignup.SetUsername(in.Username)
	user, err := h.svc.CompleteSignup(ctx, pendingSignup)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.sess.Remove(ctx, sessKeyPendingSignup)

	h.sess.Put(ctx, sessKeyUserID, user.ID)
	if err := h.sess.RenewToken(ctx); err != nil {
		h.logger.Log("msg", "renew session token", "err", err)
		h.respondErr(w, fmt.Errorf("renew session token: %w", err))
		return
	}

	h.respond(w, user, http.StatusOK)
}

func (h *handler) devLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ctx := r.Context()
	user, err := h.svc.DevLogin(ctx, in.Email)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.sess.Put(ctx, sessKeyUserID, user.ID)
	if err := h.sess.RenewToken(ctx); err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, user, http.StatusOK)
}

func (h *handler) authUser(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.AuthUser(r.Context())
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, u, http.StatusOK)
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.sess.Destroy(ctx); err != nil {
		h.respondErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if !h.sess.Exists(ctx, sessKeyUserID) {
			next.ServeHTTP(w, r)
			return
		}

		userID := h.sess.GetString(ctx, sessKeyUserID)
		ctx = context.WithValue(ctx, service.KeyAuthUserID, userID)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func genOAuthState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	h := sha256.Sum256(b)

	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(h[:]), nil
}
