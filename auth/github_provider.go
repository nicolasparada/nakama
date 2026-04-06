package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
)

func githubFetcher(ctx context.Context, conf *oauth2.Config, tok *oauth2.Token) (User, error) {
	var user User

	const baseURL = "https://api.github.com"

	client := conf.Client(ctx, tok)

	fetch := func(ctx context.Context, path string, dest any) error {
		req, err := http.NewRequestWithContext(ctx, "GET", baseURL+path, nil)
		if err != nil {
			return fmt.Errorf("create github request %q: %w", path, err)
		}

		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "nakama")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("fetch github %q: %w", path, err)
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errResp struct {
				Message string `json:"message"`
				Code    uint   `json:"code"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
				return fmt.Errorf("json decode github error response for %q: %w", path, err)
			}

			return fmt.Errorf("github error for %q: status_code=%d, message=%s, code=%d", path, resp.StatusCode, errResp.Message, errResp.Code)
		}

		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("json decode github response for %q: %w", path, err)
		}

		return nil
	}

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var emailsResp []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := fetch(gctx, "/user/emails", &emailsResp); err != nil {
			return err
		}

		var email string
		for _, e := range emailsResp {
			if e.Primary && e.Verified {
				email = strings.ToLower(e.Email)
				break
			}
		}

		if email == "" {
			return ErrEmailNotVerified
		}
		user.Email = email
		return nil
	})

	g.Go(func() error {
		var userResp struct {
			ID int64 `json:"id"`
		}
		if err := fetch(gctx, "/user", &userResp); err != nil {
			return err
		}
		user.ID = fmt.Sprintf("%d", userResp.ID)
		return nil
	})

	return user, g.Wait()
}

func NewGithubProvider(conf *oauth2.Config) *CustomProvider {
	return &CustomProvider{
		Config:   conf,
		UserFunc: githubFetcher,
	}
}
