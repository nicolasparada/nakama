package nakama

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach-go/v2/crdb"
	"github.com/hako/branca"
	"github.com/hako/durafmt"

	"github.com/nakamauwu/nakama/web"
)

// KeyAuthUserID to use in context.
const KeyAuthUserID = ctxkey("auth_user_id")

const (
	emailVerificationCodeTTL = time.Hour * 2
	authTokenTTL             = time.Hour * 24 * 14
)

var (
	// ErrInvalidRedirectURI denotes an invalid redirect URI.
	ErrInvalidRedirectURI = InvalidArgumentError("invalid redirect URI")
	// ErrUntrustedRedirectURI denotes an untrusted redirect URI.
	// That is an URI that is not in the same host as the nakama.
	ErrUntrustedRedirectURI = PermissionDeniedError("untrusted redirect URI")
	// ErrInvalidToken denotes an invalid token.
	ErrInvalidToken = InvalidArgumentError("invalid token")
	// ErrExpiredToken denotes that the token already expired.
	ErrExpiredToken = UnauthenticatedError("expired token")
	// ErrInvalidVerificationCode denotes an invalid verification code.
	ErrInvalidVerificationCode = InvalidArgumentError("invalid verification code")
	// ErrVerificationCodeNotFound denotes a not found verification code.
	ErrVerificationCodeNotFound = NotFoundError("verification code not found")
)

type ctxkey string

// TokenOutput response.
type TokenOutput struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// AuthOutput response.
type AuthOutput struct {
	User      User      `json:"user"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type SendMagicLink struct {
	UpdateEmail bool   `json:"updateEmail"`
	Email       string `json:"email"`
	RedirectURI string `json:"redirectURI"`
}

// SendMagicLink to login without passwords.
// Or to update and verify a new email address.
// A second endpoint GET /api/verify_magic_link?email&code&redirect_uri must exist.
func (s *Service) SendMagicLink(ctx context.Context, in SendMagicLink) error {
	in.Email = strings.TrimSpace(in.Email)
	in.Email = strings.ToLower(in.Email)
	if !reEmail.MatchString(in.Email) {
		return ErrInvalidEmail
	}

	_, err := s.ParseRedirectURI(in.RedirectURI)
	if err != nil {
		return err
	}

	if in.UpdateEmail {
		uid, ok := ctx.Value(KeyAuthUserID).(string)
		if !ok {
			return ErrUnauthenticated
		}

		query := `
			SELECT EXISTS (
				SELECT 1 FROM users WHERE email = $1 AND id != $2
			)
		`
		var exists bool
		row := s.DB.QueryRowContext(ctx, query, in.Email, uid)
		err := row.Scan(&exists)
		if err != nil {
			return fmt.Errorf("sql query select email existence: %w", err)
		}

		if exists {
			return ErrEmailTaken
		}
	}

	var row *sql.Row

	if in.UpdateEmail {
		uid, _ := ctx.Value(KeyAuthUserID).(string)
		query := "INSERT INTO email_verification_codes (user_id, email) VALUES ($1, $2) RETURNING code"
		row = s.DB.QueryRowContext(ctx, query, uid, in.Email)
	} else {
		query := "INSERT INTO email_verification_codes (email) VALUES ($1) RETURNING code"
		row = s.DB.QueryRowContext(ctx, query, in.Email)
	}

	var code string
	err = row.Scan(&code)
	if err != nil {
		return fmt.Errorf("could not insert verification code: %w", err)
	}

	defer func() {
		if err != nil {
			go func() {
				query := "DELETE FROM email_verification_codes WHERE email = $1 AND code = $2"
				_, err := s.DB.Exec(query, in.Email, code)
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
	err = s.magicLinkTmpl.Execute(&b, map[string]interface{}{
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
func (s *Service) VerifyMagicLink(ctx context.Context, email, code string, username *string) (AuthOutput, error) {
	var auth AuthOutput

	email = strings.TrimSpace(email)
	email = strings.ToLower(email)
	if !reEmail.MatchString(email) {
		return auth, ErrInvalidEmail
	}

	if !reUUID.MatchString(code) {
		return auth, ErrInvalidVerificationCode
	}

	if username != nil && !ValidUsername(*username) {
		return auth, ErrInvalidUsername
	}

	err := crdb.ExecuteTx(ctx, s.DB, nil, func(tx *sql.Tx) error {
		var userID sql.NullString
		var createdAt time.Time
		query := "SELECT user_id, created_at FROM email_verification_codes WHERE email = $1 AND code = $2"
		row := tx.QueryRowContext(ctx, query, email, code)
		err := row.Scan(&userID, &createdAt)
		if err == sql.ErrNoRows {
			return ErrVerificationCodeNotFound
		}

		if err != nil {
			return fmt.Errorf("could not sql query select verification code: %w", err)
		}

		if isVerificationCodeExpired(createdAt) {
			return ErrExpiredToken
		}

		// not login but update email.
		if userID.Valid {
			query := "UPDATE users SET email = $1 WHERE id = $2 RETURNING id, username, avatar"
			row = tx.QueryRowContext(ctx, query, email, userID.String)
		} else {
			query := "SELECT id, username, avatar FROM users WHERE email = $1"
			row = tx.QueryRowContext(ctx, query, email)
		}

		var avatar sql.NullString
		err = row.Scan(&auth.User.ID, &auth.User.Username, &avatar)
		if err == sql.ErrNoRows {
			if username == nil {
				return ErrUserNotFound
			}

			query := "INSERT INTO users (email, username) VALUES ($1, $2) RETURNING id"
			row := tx.QueryRowContext(ctx, query, email, username)
			err := row.Scan(&auth.User.ID)
			if isUniqueViolation(err) {
				if strings.Contains(err.Error(), "email") {
					return ErrEmailTaken
				}

				if strings.Contains(err.Error(), "username") {
					return ErrUsernameTaken
				}
			}

			if err != nil {
				return fmt.Errorf("could not sql insert user at magic link: %w", err)
			}

			auth.User.Username = *username

			return nil
		}

		if err != nil {
			return fmt.Errorf("could not sql query select user from verification code email: %w", err)
		}

		auth.User.AvatarURL = s.avatarURL(avatar)

		return nil
	})
	if err != nil {
		return auth, err
	}

	auth.ExpiresAt = time.Now().Add(authTokenTTL)
	auth.Token, err = s.codec().EncodeToString(auth.User.ID)
	if err != nil {
		return auth, fmt.Errorf("could not create auth token: %w", err)
	}

	go func() {
		_, err := s.DB.Exec("DELETE FROM email_verification_codes WHERE email = $1 AND code = $2", email, code)
		if err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not delete verification code: %w", err))
			return
		}
	}()

	return auth, nil
}

func isVerificationCodeExpired(t time.Time) bool {
	now := time.Now()
	exp := t.Add(emailVerificationCodeTTL)
	return exp.Equal(now) || exp.Before(now)
}

// DevLogin is a login for development purposes only.
// TODO: disable dev login on production.
func (s *Service) DevLogin(ctx context.Context, email string) (AuthOutput, error) {
	var out AuthOutput

	if s.DisabledDevLogin {
		return out, ErrUnimplemented
	}

	email = strings.TrimSpace(email)
	email = strings.ToLower(email)
	if !reEmail.MatchString(email) {
		return out, ErrInvalidEmail
	}

	var avatar sql.NullString
	query := "SELECT id, username, avatar FROM users WHERE email = $1"
	err := s.DB.QueryRowContext(ctx, query, email).Scan(&out.User.ID, &out.User.Username, &avatar)

	if err == sql.ErrNoRows {
		return out, ErrUserNotFound
	}

	if err != nil {
		return out, fmt.Errorf("could not query select user: %w", err)
	}

	out.User.AvatarURL = s.avatarURL(avatar)

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
			return "", ErrUnauthenticated
		}

		return "", fmt.Errorf("could not decode token: %w", err)
	}

	if !reUUID.MatchString(uid) {
		return "", ErrInvalidUserID
	}

	return uid, nil
}

// AuthUser is the current authenticated user.
func (s *Service) AuthUser(ctx context.Context) (User, error) {
	var u User
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return u, ErrUnauthenticated
	}

	return s.userByID(ctx, uid)
}

// Token to authenticate requests.
func (s *Service) Token(ctx context.Context) (TokenOutput, error) {
	var out TokenOutput
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, ErrUnauthenticated
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
