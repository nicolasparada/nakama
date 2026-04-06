package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func NewOIDCProvider(ctx context.Context, issuer string, config *oauth2.Config) (*OIDCProvider, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("create OIDC provider: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: config.ClientID})

	var emptyEndpoint oauth2.Endpoint
	if config.Endpoint == emptyEndpoint {
		config.Endpoint = provider.Endpoint()
	}

	if len(config.Scopes) == 0 {
		config.Scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	return &OIDCProvider{
		Config:   config,
		Provider: provider,
		Verifier: verifier,
	}, nil
}

type OIDCProvider struct {
	Config   *oauth2.Config
	Provider *oidc.Provider
	Verifier *oidc.IDTokenVerifier
}

func (p *OIDCProvider) AuthCodeURL(state string) string {
	return p.Config.AuthCodeURL(state)
}

func (p *OIDCProvider) User(ctx context.Context, code string) (User, error) {
	var user User

	token, err := p.Config.Exchange(ctx, code)
	if err != nil {
		return user, fmt.Errorf("exchange code for token: %w", err)
	}

	p.Config.Client(ctx, token)

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return user, errors.New("id_token not found in token response")
	}

	idToken, err := p.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return user, fmt.Errorf("verify ID token: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return user, fmt.Errorf("parse claims: %w", err)
	}

	if claims.Email == "" {
		userInfo, err := p.Provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
		if err != nil {
			return user, fmt.Errorf("fetch user info: %w", err)
		}

		claims.Email = userInfo.Email
		claims.EmailVerified = userInfo.EmailVerified
	}

	if !claims.EmailVerified {
		return user, ErrEmailNotVerified
	}

	user.ID = idToken.Subject
	user.Email = strings.ToLower(claims.Email)

	return user, nil
}
