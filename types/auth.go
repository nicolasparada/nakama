package types

import (
	"net/url"
	"strings"
	"time"

	"github.com/nicolasparada/go-errs"
)

type RequestLogin struct {
	Email       string `json:"email"`
	RedirectURI string `json:"redirectURI"`
}

func (in *RequestLogin) Validate() error {
	in.Email = strings.TrimSpace(in.Email)
	in.Email = strings.ToLower(in.Email)

	if !ValidEmail(in.Email) {
		return errs.InvalidArgumentError("invalid email")
	}

	if in.RedirectURI == "" {
		return errs.InvalidArgumentError("redirect URI is required")
	}

	if u, err := url.Parse(in.RedirectURI); err != nil || !u.IsAbs() {
		return errs.InvalidArgumentError("invalid redirect URI")
	}

	return nil
}

type RequestEmailUpdate struct {
	Email       string `json:"email"`
	RedirectURI string `json:"redirectURI"`
}

func (in *RequestEmailUpdate) Validate() error {
	in.Email = strings.TrimSpace(in.Email)
	in.Email = strings.ToLower(in.Email)

	if !ValidEmail(in.Email) {
		return errs.InvalidArgumentError("invalid email")
	}

	if in.RedirectURI == "" {
		return errs.InvalidArgumentError("redirect URI is required")
	}

	if u, err := url.Parse(in.RedirectURI); err != nil || !u.IsAbs() {
		return errs.InvalidArgumentError("invalid redirect URI")
	}

	return nil
}

type LoginResult struct {
	Status LoginStatus `json:"status"`

	// PendingSignup will be set only if Result is LoginResultPendingSignup.
	PendingSignup *PendingSignup `json:"pendingSignup,omitempty"`

	// User will be set only if Result is LoginResultSuccess.
	User *User `json:"user,omitempty"`
}

type LoginStatus string

const (
	LoginResultSuccess       LoginStatus = "success"
	LoginResultPendingSignup LoginStatus = "pending_signup"
)

type Provider struct {
	Name string
	ID   string
}

func (in *Provider) Validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.ID = strings.TrimSpace(in.ID)

	if in.Name == "" {
		return errs.InvalidArgumentError("provider name is required")
	}

	if in.ID == "" {
		return errs.InvalidArgumentError("provider ID is required")
	}

	return nil
}

type LoginWithProvider struct {
	Provider Provider
	Email    string
}

func (in *LoginWithProvider) Validate() error {
	if err := in.Provider.Validate(); err != nil {
		return err
	}

	in.Email = strings.TrimSpace(in.Email)
	in.Email = strings.ToLower(in.Email)

	if !ValidEmail(in.Email) {
		return errs.InvalidArgumentError("invalid email")
	}

	return nil
}

type PendingSignup struct {
	Provider  *Provider `json:"provider,omitempty"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`

	username *string
}

func (in *PendingSignup) SetUsername(username string) {
	username = strings.TrimSpace(username)
	in.username = &username
}

func (in PendingSignup) Username() *string {
	return in.username
}

func (in PendingSignup) IsExpired() bool {
	return time.Now().After(in.ExpiresAt)
}

func (in *PendingSignup) Validate() error {
	in.Email = strings.TrimSpace(in.Email)
	in.Email = strings.ToLower(in.Email)

	if !ValidEmail(in.Email) {
		return errs.InvalidArgumentError("invalid email")
	}

	if in.Provider != nil {
		if err := in.Provider.Validate(); err != nil {
			return err
		}

	}

	if in.username != nil {
		*in.username = strings.TrimSpace(*in.username)
		if !ValidUsername(*in.username) {
			return errs.InvalidArgumentError("invalid username")
		}
	}

	if in.IsExpired() {
		return errs.InvalidArgumentError("pending signup has expired")
	}

	return nil
}

type EmailVerificationCode struct {
	UserID    *string   `json:"userID" db:"user_id"`
	Email     string    `json:"email"`
	Hash      []byte    `json:"-" db:"hash"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

func (c EmailVerificationCode) IsExpired(ttl time.Duration) bool {
	return time.Since(c.CreatedAt) > ttl
}

type CreateEmailVerificationCode struct {
	UserID *string
	Email  string
	Hash   []byte
}
