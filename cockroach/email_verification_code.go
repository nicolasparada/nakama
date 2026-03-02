package cockroach

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/go-errs"
)

func (c *Cockroach) CreateEmailVerificationCode(ctx context.Context, in types.CreateEmailVerificationCode) (string, error) {
	const query = `
		INSERT INTO email_verification_codes (user_id, email, redirect_uri)
		VALUES (@user_id, @email, @redirect_uri)
		RETURNING code
	`
	args := pgx.StrictNamedArgs{
		"user_id":      in.UserID,
		"email":        in.Email,
		"redirect_uri": in.RedirectURI,
	}
	code, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("sql insert email verification code: %w", err)
	}

	return code, nil
}

func (c *Cockroach) EmailVerificationCode(ctx context.Context, code string) (types.EmailVerificationCode, error) {
	const query = "SELECT user_id, email, code, redirect_uri, created_at FROM email_verification_codes WHERE code = @code"
	args := pgx.StrictNamedArgs{
		"code": code,
	}
	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.EmailVerificationCode])
	if errors.Is(err, pgx.ErrNoRows) {
		return out, errs.NotFoundError("verification code not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql query select email verification code: %w", err)
	}

	return out, nil
}

func (c *Cockroach) DeleteEmailVerificationCode(ctx context.Context, code string) error {
	const query = `
		DELETE FROM email_verification_codes
		WHERE code = @code
	`
	args := pgx.StrictNamedArgs{
		"code": code,
	}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete email verification code: %w", err)
	}

	return nil
}

func (c *Cockroach) EmailVerificationCodeRedirectURI(ctx context.Context, code string) (string, error) {
	const query = "SELECT redirect_uri FROM email_verification_codes WHERE code = @code"
	args := pgx.StrictNamedArgs{
		"code": code,
	}
	redirectURI, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if db.IsNotFoundError(err) {
		return "", errs.NotFoundError("verification code not found")
	}

	if err != nil {
		return "", fmt.Errorf("sql query select email verification code redirect uri: %w", err)
	}

	return redirectURI, nil
}

func (c *Cockroach) UseEmailVerificationCode(ctx context.Context, in types.UseEmailVerificationCode, beforeDelete func(user types.User) error) (types.User, error) {
	var user types.User
	return user, c.db.RunTx(ctx, func(ctx context.Context) error {
		code, err := c.EmailVerificationCode(ctx, in.Code)
		if err != nil {
			return err
		}

		if code.IsExpired(in.TTL()) {
			return errs.UnauthenticatedError("expired token")
		}

		if code.UserID != nil {
			updated, err := c.UpdateUserEmail(ctx, *code.UserID, code.Email)
			if err != nil {
				return err
			}

			user = updated
		} else {
			exists, err := c.UserExistsWithEmail(ctx, code.Email)
			if err != nil {
				return err
			}

			if !exists {
				if in.Username == nil {
					return errs.NotFoundError("user not found")
				}

				id, err := c.CreateUser(ctx, code.Email, *in.Username)
				if err != nil {
					return err
				}

				user = types.User{
					ID:       id,
					Username: *in.Username,
				}
			} else {
				user, err = c.UserByEmail(ctx, code.Email)
				if err != nil {
					return err
				}
			}

		}

		if err := beforeDelete(user); err != nil {
			return err
		}

		return c.DeleteEmailVerificationCode(ctx, in.Code)
	})
}
