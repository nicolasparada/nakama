package cockroach

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	sqlUserProfileCols = `
		  users.id
		, users.email
		, users.username
		, users.avatar
		, users.cover
		, users.bio
		, users.waifu
		, users.husbando
		, users.followers_count
		, users.followees_count
	`
)

func (c *Cockroach) createUser(ctx context.Context, in types.CreateUser) (types.Created, error) {
	const query = `
		INSERT INTO users (email, username)
		VALUES (@email, @username)
		RETURNING id, created_at
	`
	args := pgx.StrictNamedArgs{
		"email":    in.Email,
		"username": in.Username,
	}
	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Created])
	if db.IsUniqueViolationError(err, "email") {
		return out, errs.ConflictError("email taken")
	}

	if db.IsUniqueViolationError(err, "username") {
		return out, errs.ConflictError("username taken")
	}

	if err != nil {
		return out, fmt.Errorf("sql insert user: %w", err)
	}

	return out, nil
}

func (c *Cockroach) CreateUserWithProvider(ctx context.Context, in types.ProvidedUser) (types.User, error) {
	var out types.User
	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		existsWithProvider, err := c.userExistsWithProvider(ctx, in.ProviderName, in.PrividerID)
		if err != nil {
			return err
		}

		if !existsWithProvider {
			existsWithEmail, err := c.userExistsWithEmail(ctx, in.Email)
			if err != nil {
				return err
			}

			if !existsWithEmail {
				// `user not found` is a signal to request the user for a username.
				if in.Username == nil {
					return errs.NotFoundError("user not found")
				}

				created, err := c.createUserWithProvider(ctx, in.Email, *in.Username, in.ProviderName, in.PrividerID)
				if err != nil {
					return err
				}

				out.ID = created.ID
				out.Username = *in.Username

				return nil
			}

			err = c.setProviderForUserEmail(ctx, in.Email, in.ProviderName, in.PrividerID)
			if err != nil {
				return err
			}

			user, err := c.UserByEmail(ctx, in.Email)
			if err != nil {
				return err
			}

			out = user
			return nil
		}

		user, err := c.userByProvider(ctx, in.ProviderName, in.PrividerID)
		if err != nil {
			return err
		}

		out = user
		return nil
	})
}

func (c *Cockroach) createUserWithProvider(ctx context.Context, email, username, providerName, providerID string) (types.Created, error) {
	query := fmt.Sprintf(`
		INSERT INTO users (email, username, %s_provider_id)
		VALUES (@email, @username, @provider_id)
		RETURNING id, created_at
	`, providerName)
	args := pgx.StrictNamedArgs{
		"email":       email,
		"username":    username,
		"provider_id": providerID,
	}
	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Created])
	if db.IsUniqueViolationError(err, "email") {
		return out, errs.ConflictError("email taken")
	}

	if db.IsUniqueViolationError(err, "username") {
		return out, errs.ConflictError("username taken")
	}

	if db.IsUniqueViolationError(err, providerName+"_provider_id") {
		return out, errs.ConflictError("account already linked with this provider")
	}

	if err != nil {
		return out, fmt.Errorf("sql insert user with provider: %w", err)
	}

	return out, nil
}

func (c *Cockroach) UserProfiles(ctx context.Context, in types.ListUserProfiles) (types.Page[types.UserProfile], error) {
	var out types.Page[types.UserProfile]

	args := pgx.StrictNamedArgs{}
	selects := []string{sqlUserProfileCols}
	joins := []string{}
	filters := []string{}

	if in.SearchUsername != nil {
		args["search_username"] = *in.SearchUsername
		filters = append(filters, "users.username ILIKE '%' || @search_username || '%'")
	}

	if in.ViewerID() != nil {
		args["viewer_id"] = *in.ViewerID()
		selects = append(selects,
			`users.id = @viewer_id AS me`,
			`followers.follower_id IS NOT NULL AS following`,
			`followees.followee_id IS NOT NULL AS followeed`)
		joins = append(joins,
			`LEFT JOIN follows AS followers ON followers.follower_id = @viewer_id AND followers.followee_id = users.id`,
			`LEFT JOIN follows AS followees ON followees.follower_id = users.id AND followees.followee_id = @viewer_id`)
	}

	pageArgs, err := ParsePageArgs[string](in.PageArgs)
	if err != nil {
		return out, err
	}

	if pageArgs.After != nil {
		filters = append(filters, "(users.username, users.id) < (@after_username, @after_id)")
		args["after_username"] = pageArgs.After.Value
		args["after_id"] = pageArgs.After.ID
	} else if pageArgs.Before != nil {
		filters = append(filters, "(users.username, users.id) > (@before_username, @before_id)")
		args["before_username"] = pageArgs.Before.Value
		args["before_id"] = pageArgs.Before.ID
	}

	var order, limit string
	if pageArgs.IsBackwards() {
		order = "ORDER BY users.username ASC, users.id ASC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.Last, defaultPageSize)+1) // +1 to check if there's a next page
	} else {
		order = "ORDER BY users.username DESC, users.id DESC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.First, defaultPageSize)+1) // +1 to check if there's a next page
	}

	var condWhere string
	if len(filters) > 0 {
		condWhere = " WHERE " + strings.Join(filters, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM users
		%s
		%s
		%s
		%s`,
		strings.Join(selects, ",\n\t\t"),
		strings.Join(joins, "\n\t\t"),
		condWhere,
		order,
		limit,
	)

	users, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.UserProfile])
	if err != nil {
		return out, fmt.Errorf("sql select user profiles: %w", err)
	}

	out.Items = users
	applyPageInfo(&out, pageArgs, func(up types.UserProfile) Cursor[string] {
		return Cursor[string]{ID: up.ID, Value: up.Username}
	})

	return out, nil
}

func (c *Cockroach) User(ctx context.Context, userID string) (types.User, error) {
	query := `SELECT ` + sqlUserCols + ` FROM users WHERE id = @user_id`
	args := pgx.StrictNamedArgs{"user_id": userID}
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
	args := pgx.StrictNamedArgs{"email": email}
	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.User])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if err != nil {
		return user, fmt.Errorf("sql query select user by email: %w", err)
	}

	return user, nil
}

func (c *Cockroach) userByProvider(ctx context.Context, providerName, providerID string) (types.User, error) {
	query := fmt.Sprintf(`SELECT %s FROM users WHERE %s_provider_id = @provider_id`, sqlUserCols, providerName)
	args := pgx.StrictNamedArgs{"provider_id": providerID}
	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.User])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if err != nil {
		return user, fmt.Errorf("sql query select user by provider: %w", err)
	}

	return user, nil
}

func (c *Cockroach) userExistsWithEmail(ctx context.Context, email string) (bool, error) {
	const query = "SELECT EXISTS (SELECT 1 FROM users WHERE email = @email)"
	args := pgx.StrictNamedArgs{"email": email}
	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql query select user existence by email: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) userExistsWithProvider(ctx context.Context, providerName, providerID string) (bool, error) {
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM users WHERE %s_provider_id = @provider_id)", providerName)
	args := pgx.StrictNamedArgs{"provider_id": providerID}
	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql query select user existence by provider id: %w", err)
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

func (c *Cockroach) setProviderForUserEmail(ctx context.Context, email, providerName, providerID string) error {
	query := fmt.Sprintf("UPDATE users SET %s_provider_id = @provider_id WHERE email = @email", providerName)
	args := pgx.StrictNamedArgs{
		"provider_id": providerID,
		"email":       email,
	}
	cmd, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql query update user with provider id: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return errs.NotFoundError("user not found")
	}

	return nil
}
