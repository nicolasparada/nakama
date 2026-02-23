package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nakamauwu/nakama/cockroach"
	"github.com/nakamauwu/nakama/types"
)

func (svc *Service) LoginFromProvider(ctx context.Context, name string, providedUser types.ProvidedUser) (types.User, error) {
	var u types.User

	providedUser.Email = strings.ToLower(providedUser.Email)
	if !types.ValidEmail(providedUser.Email) {
		return u, ErrInvalidEmail
	}

	if providedUser.Username != nil && !types.ValidUsername(*providedUser.Username) {
		return u, ErrInvalidUsername
	}

	err := cockroach.ExecuteTx(ctx, svc.DB, func(tx pgx.Tx) error {
		var existsWithProviderID bool

		query := fmt.Sprintf(`
			SELECT EXISTS (
				SELECT 1 FROM users WHERE %s_provider_id = $1
			)
		`, name)
		row := tx.QueryRow(ctx, query, providedUser.ID)
		err := row.Scan(&existsWithProviderID)
		if err != nil {
			return fmt.Errorf("could not sql query user existence with provider id: %w", err)
		}

		if !existsWithProviderID {
			var existsWithEmail bool

			query := `
				SELECT EXISTS (
					SELECT 1 FROM users WHERE email = $1
				)
			`
			row := tx.QueryRow(ctx, query, providedUser.Email)
			err := row.Scan(&existsWithEmail)
			if err != nil {
				return fmt.Errorf("could not sql query user existence with provider email: %w", err)
			}

			if !existsWithEmail {
				if providedUser.Username == nil {
					return ErrUserNotFound
				}

				query = fmt.Sprintf(`INSERT INTO users (email, username, %s_provider_id) VALUES ($1, $2, $3) RETURNING id`, name)
				row = tx.QueryRow(ctx, query, providedUser.Email, *providedUser.Username, providedUser.ID)
				err = row.Scan(&u.ID)
				if isUniqueViolation(err) && strings.Contains(err.Error(), "username") {
					return ErrUsernameTaken
				}

				if err != nil {
					return fmt.Errorf("could not sql insert provided user: %w", err)
				}

				u.Username = *providedUser.Username
				return nil
			}

			query = fmt.Sprintf(`UPDATE users SET %s_provider_id = $1 WHERE email = $2`, name)
			_, err = tx.Exec(ctx, query, providedUser.ID, providedUser.Email)
			if err != nil {
				return fmt.Errorf("could not sql update user with provider id: %w", err)
			}
		}

		var avatar sql.NullString
		query = fmt.Sprintf(`SELECT id, username, avatar FROM users WHERE %s_provider_id = $1`, name)
		row = tx.QueryRow(ctx, query, providedUser.ID)
		err = row.Scan(&u.ID, &u.Username, &avatar)
		if err != nil {
			return fmt.Errorf("could not sql query user by provider id: %w", err)
		}

		u.AvatarURL = svc.avatarURL(avatar)

		return nil
	})

	return u, err
}
