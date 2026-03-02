package types

import (
	"strings"
	"time"

	"github.com/nicolasparada/go-errs"
)

type TokenOutput struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

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

type ProvidedUser struct {
	ID       string
	Email    string
	Username *string
}

type EmailVerificationCode struct {
	UserID      *string   `json:"userID" db:"user_id"`
	Email       string    `json:"email"`
	Code        string    `json:"code"`
	RedirectURI string    `json:"redirectURI" db:"redirect_uri"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
}

func (c EmailVerificationCode) IsExpired(ttl time.Duration) bool {
	return time.Since(c.CreatedAt) > ttl
}

type CreateEmailVerificationCode struct {
	UserID      *string
	Email       string
	RedirectURI string
}

type UseEmailVerificationCode struct {
	Code     string
	Username *string
	ttl      time.Duration
}

func (in *UseEmailVerificationCode) SetTTL(ttl time.Duration) {
	in.ttl = ttl
}

func (in UseEmailVerificationCode) TTL() time.Duration {
	return in.ttl
}

func (in *UseEmailVerificationCode) Validate() error {
	if !ValidUUIDv4(in.Code) {
		return errs.InvalidArgumentError("invalid verification code")
	}

	if in.Username != nil {
		*in.Username = strings.TrimSpace(*in.Username)

		if !ValidUsername(*in.Username) {
			return errs.InvalidArgumentError("invalid username")
		}
	}

	return nil
}
