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
	UserID    *string   `json:"userID,omitempty" db:"user_id,omitempty"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

func (c EmailVerificationCode) IsExpired(ttl time.Duration) bool {
	return time.Since(c.CreatedAt) > ttl
}

type UseEmailVerificationCode struct {
	Email    string
	Code     string
	Username *string
}

func (in *UseEmailVerificationCode) Validate() error {
	in.Email = strings.TrimSpace(in.Email)
	in.Email = strings.ToLower(in.Email)
	if !ValidEmail(in.Email) {
		return errs.InvalidArgumentError("invalid email")
	}

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
