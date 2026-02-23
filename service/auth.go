package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/hako/branca"
	"github.com/hako/durafmt"

	"github.com/nakamauwu/nakama/types"
	"github.com/nakamauwu/nakama/web"
	"github.com/nicolasparada/go-errs"
)

var ErrUnimplemented = errors.New("unimplemented")

// KeyAuthUserID to use in context.
const KeyAuthUserID = ctxkey("auth_user_id")

const (
	emailVerificationCodeTTL = time.Hour * 2
	authTokenTTL             = time.Hour * 24 * 14
)

var (
	// ErrInvalidRedirectURI denotes an invalid redirect URI.
	ErrInvalidRedirectURI = errs.InvalidArgumentError("invalid redirect URI")
	// ErrUntrustedRedirectURI denotes an untrusted redirect URI.
	// That is an URI that is not in the same host as the service.
	ErrUntrustedRedirectURI = errs.PermissionDeniedError("untrusted redirect URI")
	// ErrInvalidToken denotes an invalid token.
	ErrInvalidToken = errs.InvalidArgumentError("invalid token")
	// ErrExpiredToken denotes that the token already expired.
	ErrExpiredToken = errs.UnauthenticatedError("expired token")
	// ErrInvalidVerificationCode denotes an invalid verification code.
	ErrInvalidVerificationCode = errs.InvalidArgumentError("invalid verification code")
	// ErrVerificationCodeNotFound denotes a not found verification code.
	ErrVerificationCodeNotFound = errs.NotFoundError("verification code not found")
)

type ctxkey string

// SendMagicLink to login without passwords.
// Or to update and verify a new email address.
// A second endpoint GET /api/verify_magic_link?email&code&redirect_uri must exist.
func (s *Service) SendMagicLink(ctx context.Context, in types.SendMagicLink) error {
	in.Email = strings.TrimSpace(in.Email)
	in.Email = strings.ToLower(in.Email)
	if !types.ValidEmail(in.Email) {
		return ErrInvalidEmail
	}

	_, err := s.ParseRedirectURI(in.RedirectURI)
	if err != nil {
		return err
	}

	if in.UpdateEmail {
		uid, ok := ctx.Value(KeyAuthUserID).(string)
		if !ok {
			return errs.Unauthenticated
		}

		exists, err := s.Cockroach.EmailTaken(ctx, in.Email, uid)
		if err != nil {
			return err
		}

		if exists {
			return ErrEmailTaken
		}
	}

	var code string
	if in.UpdateEmail {
		uid, _ := ctx.Value(KeyAuthUserID).(string)
		code, err = s.Cockroach.CreateEmailVerificationCode(ctx, in.Email, &uid)
	} else {
		code, err = s.Cockroach.CreateEmailVerificationCode(ctx, in.Email, nil)
	}
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			go func() {
				err := s.Cockroach.DeleteEmailVerificationCode(context.Background(), in.Email, code)
				if err != nil {
					_ = s.Logger.Log("error", fmt.Errorf("could not delete verification code: %w", err))
				}
			}()
		}
	}()

	// See transport/http/handler.go
	// GET /api/verify_magic_link must exist.
	magicLink := cloneURL(s.Origin)
	magicLink.Path = "/api/verify_magic_link"
	q := magicLink.Query()
	q.Set("email", in.Email)
	q.Set("verification_code", code)
	q.Set("redirect_uri", in.RedirectURI)
	magicLink.RawQuery = q.Encode()

	s.magicLinkTmplOncer.Do(func() {
		var text []byte
		text, err = web.TemplateFiles.ReadFile("template/mail/magic-link.html.tmpl")
		if err != nil {
			err = fmt.Errorf("could not read magic link template file: %w", err)
			return
		}

		s.magicLinkTmpl, err = template.
			New("mail/magic-link.html").
			Funcs(template.FuncMap{
				"human_duration": func(d time.Duration) string {
					return durafmt.Parse(d).LimitFirstN(1).String()
				},
				"html": func(s string) template.HTML {
					return template.HTML(s)
				},
			}).
			Parse(string(text))
		if err != nil {
			err = fmt.Errorf("could not parse magic link mail template: %w", err)
			return
		}
	})
	if err != nil {
		return err
	}

	var b bytes.Buffer
	err = s.magicLinkTmpl.Execute(&b, map[string]any{
		"UpdateEmail": in.UpdateEmail,
		"Origin":      s.Origin,
		"MagicLink":   magicLink,
		"TTL":         emailVerificationCodeTTL,
	})
	if err != nil {
		return fmt.Errorf("could not execute magic link mail template: %w", err)
	}

	var subject string
	if in.UpdateEmail {
		subject = "Update email at Nakama"
	} else {
		subject = "Login to Nakama"
	}
	err = s.Sender.Send(ctx, in.Email, subject, b.String(), magicLink.String())
	if err != nil {
		return fmt.Errorf("could not send magic link: %w", err)
	}

	return nil
}

// ParseRedirectURI the given redirect URI and validates it.
func (s *Service) ParseRedirectURI(rawurl string) (*url.URL, error) {
	uri, err := url.Parse(rawurl)
	if err != nil || !uri.IsAbs() {
		return nil, ErrInvalidRedirectURI
	}

	if uri.Host == s.Origin.Host || strings.HasSuffix(uri.Host, "."+s.Origin.Host) {
		return uri, nil
	}

	for _, origin := range s.AllowedOrigins {
		if strings.Contains(origin, uri.Host) {
			return uri, nil
		}
	}

	return nil, ErrUntrustedRedirectURI
}

// VerifyMagicLink checks whether the given email and verification code exists and issues a new auth token.
// If the user does not exists, it can create a new one with the given username.
func (s *Service) VerifyMagicLink(ctx context.Context, in types.UseEmailVerificationCode) (types.AuthOutput, error) {
	var out types.AuthOutput

	if err := in.Validate(); err != nil {
		return out, err
	}

	user, err := s.Cockroach.UseEmailVerificationCode(ctx, in, emailVerificationCodeTTL, func(user types.User) error {
		out.ExpiresAt = time.Now().Add(authTokenTTL)

		var err error
		out.Token, err = s.codec().EncodeToString(out.User.ID)
		if err != nil {
			return fmt.Errorf("could not create auth token: %w", err)
		}

		return nil
	})
	if err != nil {
		return out, err
	}

	user.SetAvatarURL(s.AvatarURLPrefix)
	out.User = user

	return out, nil
}

// DevLogin is a login for development purposes only.
// TODO: disable dev login on production.
func (s *Service) DevLogin(ctx context.Context, email string) (types.AuthOutput, error) {
	var out types.AuthOutput

	if s.DisabledDevLogin {
		return out, ErrUnimplemented
	}

	email = strings.TrimSpace(email)
	email = strings.ToLower(email)
	if !types.ValidEmail(email) {
		return out, ErrInvalidEmail
	}

	user, err := s.userByEmail(ctx, email)
	if err != nil {
		return out, err
	}

	out.User = user

	out.Token, err = s.codec().EncodeToString(out.User.ID)
	if err != nil {
		return out, fmt.Errorf("could not create token: %w", err)
	}

	out.ExpiresAt = time.Now().Add(authTokenTTL)

	return out, nil
}

// AuthUserIDFromToken decodes the token into a user ID.
func (s *Service) AuthUserIDFromToken(token string) (string, error) {
	uid, err := s.codec().DecodeToString(token)
	if err != nil {
		if errors.Is(err, branca.ErrInvalidToken) || errors.Is(err, branca.ErrInvalidTokenVersion) {
			return "", ErrInvalidToken
		}

		if _, ok := err.(*branca.ErrExpiredToken); ok {
			return "", ErrExpiredToken
		}

		// check branca unexported/internal chacha20poly1305 error for invalid key.
		if strings.HasSuffix(err.Error(), "authentication failed") {
			return "", errs.Unauthenticated
		}

		return "", fmt.Errorf("could not decode token: %w", err)
	}

	if !types.ValidUUIDv4(uid) {
		return "", ErrInvalidUserID
	}

	return uid, nil
}

// AuthUser is the current authenticated user.
func (s *Service) AuthUser(ctx context.Context) (types.User, error) {
	var u types.User
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return u, errs.Unauthenticated
	}

	return s.userByID(ctx, uid)
}

// Token to authenticate requests.
func (s *Service) Token(ctx context.Context) (types.TokenOutput, error) {
	var out types.TokenOutput
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, errs.Unauthenticated
	}

	var err error
	out.Token, err = s.codec().EncodeToString(uid)
	if err != nil {
		return out, fmt.Errorf("could not create token: %w", err)
	}

	out.ExpiresAt = time.Now().Add(authTokenTTL)

	return out, nil
}

func (s *Service) codec() *branca.Branca {
	cdc := branca.NewBranca(s.TokenKey)
	cdc.SetTTL(uint32(authTokenTTL.Seconds()))
	return cdc
}
