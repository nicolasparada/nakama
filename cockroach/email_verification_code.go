package cockroach

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

func (c *Cockroach) CreateEmailVerificationCode(ctx context.Context, email string, userID *string) (string, error) {
	const query = `
		INSERT INTO email_verification_codes (user_id, email)
		VALUES (@user_id, @email)
		RETURNING code
	`
	args := pgx.StrictNamedArgs{
		"user_id": userID,
		"email":   email,
	}
	code, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("sql insert email verification code: %w", err)
	}

	return code, nil
}

func (c *Cockroach) EmailVerificationCode(ctx context.Context, email, code string) (types.EmailVerificationCode, error) {
	const query = "SELECT user_id, created_at FROM email_verification_codes WHERE email = @email AND code = @code"
	args := pgx.StrictNamedArgs{
		"email": email,
		"code":  code,
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

func (c *Cockroach) DeleteEmailVerificationCode(ctx context.Context, email, code string) error {
	const query = `
		DELETE FROM email_verification_codes
		WHERE email = @email AND code = @code
	`
	args := pgx.StrictNamedArgs{
		"email": email,
		"code":  code,
	}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete email verification code: %w", err)
	}

	return nil
}

func (c *Cockroach) UseEmailVerificationCode(ctx context.Context, in types.UseEmailVerificationCode, codeTTL time.Duration, beforeDelete func(user types.User) error) (types.User, error) {
	var user types.User
	return user, c.db.RunTx(ctx, func(ctx context.Context) error {
		code, err := c.EmailVerificationCode(ctx, in.Email, in.Code)
		if err != nil {
			return err
		}

		if code.IsExpired(codeTTL) {
			return errs.UnauthenticatedError("expired token")
		}

		if code.UserID != nil {
			updated, err := c.UpdateUserEmail(ctx, *code.UserID, in.Email)
			if err != nil {
				return err
			}

			user = updated
		} else {
			exists, err := c.UserExistsWithEmail(ctx, in.Email)
			if err != nil {
				return err
			}

			if !exists {
				if in.Username == nil {
					return errs.NotFoundError("user not found")
				}

				id, err := c.CreateUser(ctx, in.Email, *in.Username)
				if err != nil {
					return err
				}

				user = types.User{
					ID:       id,
					Username: *in.Username,
				}
			} else {
				user, err = c.UserByEmail(ctx, in.Email)
				if err != nil {
					return err
				}
			}

		}

		if err := beforeDelete(user); err != nil {
			return err
		}

		return c.DeleteEmailVerificationCode(ctx, in.Email, in.Code)
	})
}
