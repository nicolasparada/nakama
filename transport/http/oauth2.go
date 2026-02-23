package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-kit/log/level"
	"github.com/hybridtheory/samesite-cookie-support"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"

	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/types"
	webtemplate "github.com/nakamauwu/nakama/web"
	"github.com/nicolasparada/go-errs"
)

const oauth2Timeout = time.Minute * 2

var refreshTmpl = template.Must(template.ParseFS(webtemplate.TemplateFiles, "template/refresh.html.tmpl"))

type OauthProvider struct {
	Name            string
	Config          *oauth2.Config
	FetchUser       func(ctx context.Context, config *oauth2.Config, token *oauth2.Token) (types.ProvidedUser, error)
	IDTokenVerifier *oidc.IDTokenVerifier
}

var GithubUserFetcher = func(ctx context.Context, config *oauth2.Config, token *oauth2.Token) (types.ProvidedUser, error) {
	const baseURL = "https://api.github.com"

	var user struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		req, err := http.NewRequestWithContext(gctx, http.MethodGet, baseURL+"/user", nil)
		if err != nil {
			return fmt.Errorf("could not create user request: %w", err)
		}

		req.Header.Set("User-Agent", "nakama")

		resp, err := config.Client(ctx, token).Do(req)
		if err != nil {
			return errServiceUnavailable
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return errServiceUnavailable
		}

		err = json.NewDecoder(resp.Body).Decode(&user)
		if err != nil {
			return errServiceUnavailable
		}

		return nil
	})

	g.Go(func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/user/emails", nil)
		if err != nil {
			return fmt.Errorf("could not create user emails request: %w", err)
		}

		req.Header.Set("User-Agent", "nakama")

		resp, err := config.Client(ctx, token).Do(req)
		if err != nil {
			return errServiceUnavailable
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return errServiceUnavailable
		}

		err = json.NewDecoder(resp.Body).Decode(&emails)
		if err != nil {
			return errServiceUnavailable
		}

		return nil
	})

	var out types.ProvidedUser

	if err := g.Wait(); err != nil {
		return out, err
	}

	for _, email := range emails {
		if email.Email != "" && email.Verified && email.Primary {
			out.Email = email.Email
			break
		}
	}

	if out.Email == "" {
		for _, email := range emails {
			if email.Email != "" && email.Verified {
				out.Email = email.Email
				break
			}
		}
	}

	if out.Email == "" {
		return out, errEmailNotProvided
	}

	out.ID = fmt.Sprintf("%d", user.ID)
	out.Username = &user.Login

	return out, nil
}

func (h *handler) oauth2Handler(provider OauthProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		redirectURIString := q.Get("redirect_uri")
		if redirectURIString == "" {
			redirectURIString = r.Referer()
		}
		redirectURI, err := h.svc.ParseRedirectURI(redirectURIString)
		if err != nil {
			h.respondErr(w, err)
			return
		}

		username := q.Get("username")
		if username != "" && !types.ValidUsername(username) {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{service.ErrInvalidUsername.Error()},
			}, http.StatusSeeOther)
			return
		}

		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			_ = h.logger.Log("err", fmt.Errorf("could not generate oauth2 state: %w", err))
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{"internal server error"},
			}, http.StatusSeeOther)
			return
		}

		state := base64.RawStdEncoding.EncodeToString(b)

		stateValue, err := h.cookieCodec.Encode("oauth2_state", state)
		if err != nil {
			_ = h.logger.Log("err", fmt.Errorf("could not cookie encode oauth2 state: %w", err))
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{"internal server error"},
			}, http.StatusSeeOther)
			return
		}

		{
			cookie := &http.Cookie{
				Name:     "oauth2_state",
				Value:    stateValue,
				Expires:  time.Now().Add(oauth2Timeout),
				Secure:   h.origin.Scheme == "https",
				HttpOnly: true,
			}
			if samesite.IsSameSiteCookieSupported(r.UserAgent()) {
				cookie.SameSite = http.SameSiteLaxMode
			}
			http.SetCookie(w, cookie)
		}

		{
			cookie := &http.Cookie{
				Name:     "oauth2_redirect_uri",
				Value:    redirectURI.String(),
				Expires:  time.Now().Add(oauth2Timeout),
				Secure:   h.origin.Scheme == "https",
				HttpOnly: true,
			}
			if samesite.IsSameSiteCookieSupported(r.UserAgent()) {
				cookie.SameSite = http.SameSiteLaxMode
			}
			http.SetCookie(w, cookie)
		}

		{
			cookie := &http.Cookie{
				Name:     "oauth2_username",
				Value:    username,
				Expires:  time.Now().Add(oauth2Timeout),
				Secure:   h.origin.Scheme == "https",
				HttpOnly: true,
			}
			if samesite.IsSameSiteCookieSupported(r.UserAgent()) {
				cookie.SameSite = http.SameSiteLaxMode
			}
			http.SetCookie(w, cookie)
		}

		u := provider.Config.AuthCodeURL(state)
		http.Redirect(w, r, u, http.StatusTemporaryRedirect)
	}
}

func (h *handler) oauth2CallbackHandler(provider OauthProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redirectURICookie, err := r.Cookie("oauth2_redirect_uri")
		if err == http.ErrNoCookie {
			// Try client side page refresh to reset lost cookies.
			if !r.URL.Query().Has("did_refresh") { // prevent infinite loop.
				u, err := url.Parse(r.RequestURI)
				if err != nil {
					h.respondErr(w, fmt.Errorf("could not parse request uri: %w", err))
					return
				}

				q := u.Query()
				q.Set("did_refresh", "true")
				u.RawQuery = q.Encode()

				var buff bytes.Buffer
				err = refreshTmpl.Execute(&buff, u)
				if err != nil {
					h.respondErr(w, fmt.Errorf("could not render refresh template: %w", err))
					return
				}

				r.Header.Set("Refresh", "0; url="+url.QueryEscape(u.String()))
				w.Header().Set("Content-Type", "text/html; charset=utf-u")
				_, err = w.Write(buff.Bytes())
				if err != nil && !errors.Is(err, context.Canceled) {
					_ = level.Error(h.logger).Log("msg", "could not write http response", "err", err)
				}
			}

			h.respondErr(w, errOauthTimeout)
			return
		}

		if err != nil {
			h.respondErr(w, errTeaPot)
			return
		}

		redirectURI, err := h.svc.ParseRedirectURI(redirectURICookie.Value)
		if err != nil {
			h.respondErr(w, err)
			return
		}

		usernameCookie, err := r.Cookie("oauth2_username")
		if err == http.ErrNoCookie {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{errOauthTimeout.Error()},
			}, http.StatusSeeOther)
			return
		}

		if err != nil {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{errTeaPot.Error()},
			}, http.StatusSeeOther)
			return
		}

		stateCookie, err := r.Cookie("oauth2_state")
		if err == http.ErrNoCookie {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{errOauthTimeout.Error()},
			}, http.StatusSeeOther)
			return
		}

		if err != nil {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{errTeaPot.Error()},
			}, http.StatusSeeOther)
			return
		}

		var state string
		err = h.cookieCodec.Decode("oauth2_state", stateCookie.Value, &state)
		if err != nil {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{errTeaPot.Error()},
			}, http.StatusSeeOther)
			return
		}

		q := r.URL.Query()
		if q.Get("state") != state {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{errTeaPot.Error()},
			}, http.StatusSeeOther)
			return
		}

		ctx := r.Context()
		token, err := provider.Config.Exchange(ctx, q.Get("code"))
		if err != nil {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{errs.Unauthenticated.Error()},
			}, http.StatusSeeOther)
			return
		}

		var providedUser types.ProvidedUser

		if provider.IDTokenVerifier != nil {
			rawIDToken, ok := token.Extra("id_token").(string)
			if !ok {
				redirectWithHashFragment(w, r, redirectURI, url.Values{
					"error": []string{errs.Unauthenticated.Error()},
				}, http.StatusSeeOther)
				return
			}

			idToken, err := provider.IDTokenVerifier.Verify(ctx, rawIDToken)
			if err != nil {
				redirectWithHashFragment(w, r, redirectURI, url.Values{
					"error": []string{errs.Unauthenticated.Error()},
				}, http.StatusSeeOther)
				return
			}

			var claims struct {
				Sub      string `json:"sub"`
				Email    string `json:"email"`
				Verified bool   `json:"email_verified"`
			}
			if err := idToken.Claims(&claims); err != nil {
				redirectWithHashFragment(w, r, redirectURI, url.Values{
					"error": []string{errs.Unauthenticated.Error()},
				}, http.StatusSeeOther)
				return
			}

			if !claims.Verified {
				redirectWithHashFragment(w, r, redirectURI, url.Values{
					"error": []string{errEmailNotVerified.Error()},
				}, http.StatusSeeOther)
				return
			}

			if claims.Email == "" {
				redirectWithHashFragment(w, r, redirectURI, url.Values{
					"error": []string{errEmailNotProvided.Error()},
				}, http.StatusSeeOther)
				return
			}

			providedUser.ID = claims.Sub
			providedUser.Email = claims.Email
		} else {
			var err error
			providedUser, err = provider.FetchUser(ctx, provider.Config, token)
			if err != nil {
				statusCode := err2code(err)
				if statusCode != http.StatusInternalServerError {
					redirectWithHashFragment(w, r, redirectURI, url.Values{
						"error": []string{err.Error()},
					}, http.StatusSeeOther)
					return
				}

				if !errors.Is(err, context.Canceled) {
					_ = h.logger.Log("err", err)
				}
				redirectWithHashFragment(w, r, redirectURI, url.Values{
					"error": []string{"internal server error"},
				}, http.StatusSeeOther)
				return
			}
		}

		if usernameCookie.Value != "" {
			s := usernameCookie.Value
			providedUser.Username = &s
		}

		user, err := h.svc.LoginFromProvider(ctx, provider.Name, providedUser)
		if err == service.ErrUserNotFound || err == service.ErrInvalidUsername || err == service.ErrUsernameTaken {
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error":          []string{err.Error()},
				"retry_endpoint": []string{"/api/" + provider.Name + "_auth"},
			}, http.StatusSeeOther)
			return
		}

		if err != nil {
			statusCode := err2code(err)
			if statusCode != http.StatusInternalServerError {
				redirectWithHashFragment(w, r, redirectURI, url.Values{
					"error": []string{err.Error()},
				}, http.StatusSeeOther)
				return
			}

			if !errors.Is(err, context.Canceled) {
				_ = h.logger.Log("err", err)
			}
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{"internal server error"},
			}, http.StatusSeeOther)
			return
		}

		ctx = context.WithValue(ctx, service.KeyAuthUserID, user.ID)
		auth, err := h.svc.Token(ctx)
		if err != nil {
			statusCode := err2code(err)
			if statusCode != http.StatusInternalServerError {
				redirectWithHashFragment(w, r, redirectURI, url.Values{
					"error": []string{err.Error()},
				}, http.StatusSeeOther)
				return
			}

			if !errors.Is(err, context.Canceled) {
				_ = h.logger.Log("err", err)
			}
			redirectWithHashFragment(w, r, redirectURI, url.Values{
				"error": []string{"internal server error"},
			}, http.StatusSeeOther)
			return
		}

		values := url.Values{
			"token":         []string{auth.Token},
			"expires_at":    []string{auth.ExpiresAt.Format(time.RFC3339Nano)},
			"user.id":       []string{user.ID},
			"user.username": []string{user.Username},
		}
		if user.AvatarURL != nil {
			values.Set("user.avatar_url", *user.AvatarURL)
		}
		redirectWithHashFragment(w, r, redirectURI, values, http.StatusSeeOther)
	}
}
