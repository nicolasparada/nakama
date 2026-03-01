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

const (
	sqlUserCols = `
		  users.id
		, users.username
		, users.avatar
	`
	// The JSON version uses `avatarURL` instead of `avatar`.
	sqlUserJSONB = `
		jsonb_build_object(
			'id', users.id,
			'username', users.username,
			'avatarURL', users.avatar
		) AS user
	`
)

func (c *Cockroach) CreateUser(ctx context.Context, email, username string) (string, error) {
	const query = `
		INSERT INTO users (email, username)
		VALUES (@email, @username)
		RETURNING id
	`
	args := pgx.StrictNamedArgs{
		"email":    email,
		"username": username,
	}
	id, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if db.IsUniqueViolationError(err, "email") {
		return "", errs.ConflictError("email taken")
	}

	if db.IsUniqueViolationError(err, "username") {
		return "", errs.ConflictError("username taken")
	}

	if err != nil {
		return "", fmt.Errorf("sql insert user: %w", err)
	}

	return id, nil
}

func (c *Cockroach) User(ctx context.Context, userID string) (types.User, error) {
	query := `SELECT ` + sqlUserCols + ` FROM users WHERE id = @user_id`
	args := pgx.StrictNamedArgs{
		"user_id": userID,
	}
	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.User])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if err != nil {
		return user, fmt.Errorf("sql query select user: %w", err)
	}

	return user, nil
}

func (c *Cockroach) UserByEmail(ctx context.Context, email string) (types.User, error) {
	query := `SELECT ` + sqlUserCols + ` FROM users WHERE email = @email`
	args := pgx.StrictNamedArgs{
		"email": email,
	}
	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.User])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if err != nil {
		return user, fmt.Errorf("sql query select user by email: %w", err)
	}

	return user, nil
}

func (c *Cockroach) UserExistsWithEmail(ctx context.Context, email string) (bool, error) {
	const query = "SELECT EXISTS (SELECT 1 FROM users WHERE email = @email)"
	args := pgx.StrictNamedArgs{
		"email": email,
	}
	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql query select user existence by email: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) EmailTaken(ctx context.Context, email, userID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM users WHERE email = @email AND id != @user_id
		)
	`
	args := pgx.StrictNamedArgs{
		"email":   email,
		"user_id": userID,
	}
	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql query select email taken: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) UpdateUserEmail(ctx context.Context, userID, email string) (types.User, error) {
	const query = "UPDATE users SET email = @email WHERE id = @user_id RETURNING " + sqlUserCols
	args := pgx.StrictNamedArgs{
		"email":   email,
		"user_id": userID,
	}
	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.User])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if err != nil {
		return user, fmt.Errorf("sql query update user email: %w", err)
	}

	return user, nil
}
