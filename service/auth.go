package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/earthboundkid/crockford/v2"
	"github.com/nakamauwu/nakama/types"
	"github.com/nakamauwu/nakama/web"
	"github.com/nicolasparada/go-errs"
)

var tmplLoginEmail = template.Must(template.New("login-email.tmpl").Funcs(emailTemplateFuncs).ParseFS(web.TemplateFiles, "template/login-email.tmpl"))

type TemplDataLoginEmail struct {
	MagicLink *url.URL
	Code      string
	TTL       time.Duration
}

var ErrUnimplemented = errors.New("unimplemented")

// KeyAuthUserID to use in context.
const KeyAuthUserID = ctxkey("auth_user_id")

const verifyEmailTTL = time.Minute * 15

const (
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

func (svc *Service) OAuthURL(ctx context.Context, providerName string, state string) (string, error) {
	provider, ok := svc.AuthProviders.Get(providerName)
	if !ok {
		return "", errs.InvalidArgumentError("unknown provider")
	}

	return provider.AuthCodeURL(state), nil
}

func (svc *Service) OAuthLogin(ctx context.Context, providerName string, code string) (types.LoginResult, error) {
	var resp types.LoginResult

	code = strings.TrimSpace(code)
	if code == "" {
		return resp, errs.InvalidArgumentError("code is required")
	}

	provider, ok := svc.AuthProviders.Get(providerName)
	if !ok {
		return resp, errs.InvalidArgumentError("unknown provider")
	}

	providedUser, err := provider.User(ctx, code)
	if err != nil {
		return resp, fmt.Errorf("get user from provider: %w", err)
	}

	user, err := svc.Cockroach.UserByProviderOrEmail(ctx, types.Provider{
		Name: providerName,
		ID:   providedUser.ID,
	}, providedUser.Email)
	if errors.Is(err, errs.NotFound) {
		return types.LoginResult{
			Status: types.LoginResultPendingSignup,
			PendingSignup: &types.PendingSignup{
				Provider: &types.Provider{
					Name: providerName,
					ID:   providedUser.ID,
				},
				Email:     providedUser.Email,
				ExpiresAt: time.Now().Add(verifyEmailTTL),
			},
		}, nil
	}

	if err != nil {
		return resp, err
	}

	return types.LoginResult{
		Status: types.LoginResultSuccess,
		User:   &user,
	}, nil
}

// TODO: Apply rate-limit per email or even IP to prevent abuse of the login endpoint.
func (s *Service) RequestLogin(ctx context.Context, in types.RequestLogin) error {
	if err := in.Validate(); err != nil {
		return err
	}

	magicLink, err := url.Parse(in.RedirectURI)
	// it should be already validated, but just to keep code style consistent.
	if err != nil || !magicLink.IsAbs() {
		return errs.InvalidArgumentError("invalid redirect URI")
	}

	plainText, hash, err := generateVerificationCode()
	if err != nil {
		return fmt.Errorf("generate verification code: %w", err)
	}

	err = s.Cockroach.CreateEmailVerificationCode(ctx, types.CreateEmailVerificationCode{
		Email: in.Email,
		Hash:  hash,
	})
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			go func() {
				err := s.Cockroach.DeleteEmailVerificationCode(context.Background(), hash)
				if err != nil {
					_ = s.Logger.Log("error", fmt.Errorf("delete verification code: %w", err))
				}
			}()
		}
	}()

	q := magicLink.Query()
	q.Set("code", plainText)
	magicLink.RawQuery = q.Encode()

	data := TemplDataLoginEmail{
		MagicLink: magicLink,
		Code:      plainText,
		TTL:       verifyEmailTTL,
	}

	var buf bytes.Buffer
	if err := tmplLoginEmail.Execute(&buf, data); err != nil {
		return fmt.Errorf("render login email template: %w", err)
	}

	// TODO: use outbox pattern to send email asynchronously and retry on failure.
	return s.Sender.Send(ctx, in.Email, "Login to Nakama", buf.String(), fmt.Sprintf(
		`Use this link to login to Nakama. The link is valid for %s.\n\n`+
			`%s\n\n`+
			`Or copy and paste this code in the app: %s`,
		verifyEmailTTL,
		magicLink,
		plainText,
	))
}

func (s *Service) VerifyLogin(ctx context.Context, code string) (types.LoginResult, error) {
	var resp types.LoginResult

	code = strings.TrimSpace(code)
	if code == "" {
		return resp, errs.InvalidArgumentError("verification code is required")
	}

	hash := verificationCodeHash(code)

	return resp, s.Cockroach.UseEmailVerificationCode(ctx, hash, func(ctx context.Context, verificationCode types.EmailVerificationCode) (bool, error) {
		if verificationCode.IsExpired(verifyEmailTTL) {
			return true, errs.UnauthenticatedError("expired verification code")
		}

		user, err := s.userByEmail(ctx, verificationCode.Email)
		if errors.Is(err, errs.NotFound) {
			resp = types.LoginResult{
				Status: types.LoginResultPendingSignup,
				PendingSignup: &types.PendingSignup{
					Email:     verificationCode.Email,
					ExpiresAt: time.Now().Add(verifyEmailTTL),
				},
			}
			return true, nil
		}

		if err != nil {
			return false, err
		}

		resp = types.LoginResult{
			Status: types.LoginResultSuccess,
			User:   &user,
		}
		return true, nil
	})
}

func (svc *Service) CompleteSignup(ctx context.Context, in types.PendingSignup) (types.User, error) {
	var user types.User

	if err := in.Validate(); err != nil {
		return user, err
	}

	if in.Username() == nil {
		return user, errs.InvalidArgumentError("username is required")
	}

	created, err := svc.Cockroach.CreateUser(ctx, types.CreateUser{
		Email:    in.Email,
		Username: *in.Username(),
		Provider: in.Provider,
	})
	if err != nil {
		return user, err
	}

	return svc.userByID(ctx, created.ID)
}

func (s *Service) DevLogin(ctx context.Context, email string) (types.User, error) {
	var out types.User

	if s.DisabledDevLogin {
		return out, ErrUnimplemented
	}

	email = strings.TrimSpace(email)
	email = strings.ToLower(email)
	if !types.ValidEmail(email) {
		return out, errs.InvalidArgumentError("invalid email")
	}

	return s.userByEmail(ctx, email)
}

func (s *Service) AuthUser(ctx context.Context) (types.User, error) {
	var u types.User
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return u, errs.Unauthenticated
	}

	return s.userByID(ctx, uid)
}

func generateVerificationCode() (string, []byte, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate random bytes: %w", err)
	}

	plainText := crockford.Upper.EncodeToString(b)

	hash := sha256.Sum256([]byte(plainText))
	return plainText, hash[:], nil
}

func verificationCodeHash(plainText string) []byte {
	hash := sha256.Sum256([]byte(plainText))
	return hash[:]
}
