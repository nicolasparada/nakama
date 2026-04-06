package auth

import (
	"context"
	"errors"
	"sync"
)

var ErrEmailNotVerified = errors.New("email not verified")

type User struct {
	ID    string
	Email string
}

type Provider interface {
	AuthCodeURL(state string) string
	User(ctx context.Context, code string) (User, error)
}

type Providers struct {
	data map[string]Provider
	mu   sync.RWMutex
}

func MakeProviders() *Providers {
	return &Providers{
		data: make(map[string]Provider),
	}
}

func (p *Providers) Register(name string, provider Provider) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data[name] = provider
}

func (p *Providers) Get(name string) (Provider, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	provider, ok := p.data[name]
	return provider, ok
}
