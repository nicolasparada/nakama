package cockroach

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

func (c *Cockroach) CreateEmailVerificationCode(ctx context.Context, in types.CreateEmailVerificationCode) error {
	const query = `
		INSERT INTO email_verification_codes (user_id, email, hash)
		VALUES (@user_id, @email, @hash)
	`
	args := pgx.StrictNamedArgs{
		"user_id": in.UserID,
		"email":   in.Email,
		"hash":    in.Hash,
	}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql insert email verification code: %w", err)
	}

	return nil
}

func (c *Cockroach) EmailVerificationCode(ctx context.Context, hash []byte) (types.EmailVerificationCode, error) {
	const query = "SELECT user_id, email, hash, created_at FROM email_verification_codes WHERE hash = @hash"
	args := pgx.StrictNamedArgs{
		"hash": hash,
	}
	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.EmailVerificationCode])
	if errors.Is(err, pgx.ErrNoRows) {
		return out, errs.NotFoundError("verification code not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql select email verification code: %w", err)
	}

	return out, nil
}

func (c *Cockroach) DeleteEmailVerificationCode(ctx context.Context, hash []byte) error {
	const query = `
		DELETE FROM email_verification_codes
		WHERE hash = @hash
	`
	args := pgx.StrictNamedArgs{
		"hash": hash,
	}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete email verification code: %w", err)
	}

	return nil
}

func (c *Cockroach) UseEmailVerificationCode(ctx context.Context, hash []byte, deleteFunc func(ctx context.Context, verificationCode types.EmailVerificationCode) (bool, error)) error {
	// errDeleteFunc is returned outside of the tx, so it doesn't cause the tx to rollback.
	var errDeleteFunc error
	err := c.db.RunTx(ctx, func(ctx context.Context) error {
		verificationCode, err := c.EmailVerificationCode(ctx, hash)
		if err != nil {
			return err
		}

		var shouldDelete bool
		shouldDelete, errDeleteFunc = deleteFunc(ctx, verificationCode)
		if shouldDelete {
			if err := c.DeleteEmailVerificationCode(ctx, hash); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return errDeleteFunc
}
