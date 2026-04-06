package auth

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
)

type UserFetcher func(ctx context.Context, config *oauth2.Config, tok *oauth2.Token) (User, error)

type CustomProvider struct {
	Config   *oauth2.Config
	UserFunc UserFetcher
}

func (p *CustomProvider) AuthCodeURL(state string) string {
	return p.Config.AuthCodeURL(state)
}

func (p *CustomProvider) User(ctx context.Context, code string) (User, error) {
	var user User

	token, err := p.Config.Exchange(ctx, code)
	if err != nil {
		return user, fmt.Errorf("exchange code for token: %w", err)
	}

	return p.UserFunc(ctx, p.Config, token)
}
