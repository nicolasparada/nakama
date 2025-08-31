package cockroach

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

var userColumns = [...]string{
	"users.id",
	"users.email",
	"users.username",
	"users.avatar",
	"users.followers_count",
	"users.following_count",
	"users.created_at",
	"users.updated_at",
}

var userColumnsStr = strings.Join(userColumns[:], ", ")

func (c *Cockroach) UpsertUser(ctx context.Context, in types.UpsertUser) (types.User, error) {
	var out types.User

	// TODO: check user uniqueness case-insensitively.

	query := `
		INSERT INTO users (id, email, username)
		VALUES (@user_id, LOWER(@email), @username)
		ON CONFLICT (email) DO UPDATE
		SET username = EXCLUDED.username, updated_at = NOW()
		RETURNING ` + userColumnsStr + `
	`

	rows, err := c.db.Query(ctx, query, pgx.StrictNamedArgs{
		"user_id":  id.Generate(),
		"email":    in.Email,
		"username": in.Username,
	})
	if err != nil {
		return out, fmt.Errorf("sql upsert user: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.User])
	if err != nil {
		return out, fmt.Errorf("sql collect upserted user: %w", err)
	}

	return out, nil
}

func (c *Cockroach) User(ctx context.Context, in types.RetrieveUser) (types.User, error) {
	var out types.User

	query := `
		SELECT 
			` + userColumnsStr + `,
			CASE WHEN @logged_in_user_id::VARCHAR IS NOT NULL THEN
				JSON_BUILD_OBJECT(
					'followsYou', EXISTS(SELECT 1 FROM follows WHERE follower_id = users.id AND followee_id = @logged_in_user_id),
					'followedByYou', EXISTS(SELECT 1 FROM follows WHERE follower_id = @logged_in_user_id AND followee_id = users.id),
					'isMe', users.id = @logged_in_user_id
				)
			ELSE
				JSON_BUILD_OBJECT(
					'followsYou', false,
					'followedByYou', false,
					'isMe', false
				)
			END AS relationship
		FROM users
		WHERE users.id = @user_id
	`

	rows, err := c.db.Query(ctx, query, pgx.StrictNamedArgs{
		"user_id":           in.UserID,
		"logged_in_user_id": in.LoggedInUserID(),
	})
	if err != nil {
		return out, fmt.Errorf("sql select user: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.User])
	if db.IsNotFoundError(err) {
		return out, errs.NewNotFoundError("user not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql collect selected user: %w", err)
	}

	return out, nil
}

func (c *Cockroach) UserFromUsername(ctx context.Context, in types.RetrieveUserFromUsername) (types.User, error) {
	var out types.User

	query := `
		SELECT 
			` + userColumnsStr + `,
			CASE WHEN @logged_in_user_id::VARCHAR IS NOT NULL THEN
				JSON_BUILD_OBJECT(
					'followsYou', EXISTS(SELECT 1 FROM follows WHERE follower_id = users.id AND followee_id = @logged_in_user_id),
					'followedByYou', EXISTS(SELECT 1 FROM follows WHERE follower_id = @logged_in_user_id AND followee_id = users.id),
					'isMe', users.id = @logged_in_user_id
				)
			ELSE
				JSON_BUILD_OBJECT(
					'followsYou', false,
					'followedByYou', false,
					'isMe', false
				)
			END AS relationship
		FROM users
		WHERE LOWER(users.username) = LOWER(@username)
	`

	rows, err := c.db.Query(ctx, query, pgx.StrictNamedArgs{
		"username":          in.Username,
		"logged_in_user_id": in.LoggedInUserID(),
	})
	if err != nil {
		return out, fmt.Errorf("sql select user by username: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.User])
	if db.IsNotFoundError(err) {
		return out, errs.NewNotFoundError("user not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql collect selected user by username: %w", err)
	}

	return out, nil
}

func (c *Cockroach) ToggleFollow(ctx context.Context, in types.ToggleFollow) (inserted bool, err error) {
	return inserted, c.db.RunTx(ctx, func(ctx context.Context) error {
		exists, err := c.followExists(ctx, in.LoggedInUserID(), in.FolloweeID)
		if err != nil {
			return err
		}

		if exists {
			return c.deleteFollow(ctx, in.LoggedInUserID(), in.FolloweeID)
		}

		inserted = true
		return c.insertFollow(ctx, in.LoggedInUserID(), in.FolloweeID)
	})
}

func (c *Cockroach) followExists(ctx context.Context, followerID, followeeID string) (bool, error) {
	var exists bool
	err := c.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM follows 
			WHERE follower_id = @follower_id AND followee_id = @followee_id
		)
	`, pgx.StrictNamedArgs{
		"follower_id": followerID,
		"followee_id": followeeID,
	}).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sql check follow exists: %w", err)
	}
	return exists, nil
}

func (c *Cockroach) insertFollow(ctx context.Context, followerID, followeeID string) error {
	_, err := c.db.Exec(ctx, `
		INSERT INTO follows (follower_id, followee_id) 
		VALUES (@follower_id, @followee_id)
	`, pgx.StrictNamedArgs{
		"follower_id": followerID,
		"followee_id": followeeID,
	})
	if err != nil {
		return fmt.Errorf("sql insert follow: %w", err)
	}
	return nil
}

func (c *Cockroach) deleteFollow(ctx context.Context, followerID, followeeID string) error {
	_, err := c.db.Exec(ctx, `
		DELETE FROM follows 
		WHERE follower_id = @follower_id AND followee_id = @followee_id
	`, pgx.StrictNamedArgs{
		"follower_id": followerID,
		"followee_id": followeeID,
	})
	if err != nil {
		return fmt.Errorf("sql delete follow: %w", err)
	}
	return nil
}

func (c *Cockroach) UpdateUserAvatar(ctx context.Context, in types.UpdateUserAvatar) error {
	const q = `
		UPDATE users
		SET avatar = @avatar, updated_at = NOW()
		WHERE id = @user_id
	`

	_, err := c.db.Exec(ctx, q, pgx.StrictNamedArgs{
		"user_id": in.UserID,
		"avatar":  in.Avatar,
	})
	if err != nil {
		return fmt.Errorf("sql update user avatar: %w", err)
	}

	return nil
}

func (c *Cockroach) SearchUsers(ctx context.Context, in types.SearchUsers) (types.SimplePage[types.User], error) {
	var out types.SimplePage[types.User]

	query := `
		SELECT 
			` + userColumnsStr + `,
			CASE WHEN @logged_in_user_id::VARCHAR IS NOT NULL THEN
				JSON_BUILD_OBJECT(
					'followsYou', EXISTS(SELECT 1 FROM follows WHERE follower_id = users.id AND followee_id = @logged_in_user_id),
					'followedByYou', EXISTS(SELECT 1 FROM follows WHERE follower_id = @logged_in_user_id AND followee_id = users.id),
					'isMe', users.id = @logged_in_user_id
				)
			ELSE
				JSON_BUILD_OBJECT(
					'followsYou', false,
					'followedByYou', false,
					'isMe', false
				)
			END AS relationship
		FROM users
		WHERE (
			LOWER(users.username) LIKE LOWER('%' || @query || '%')
			OR
			similarity(users.username, @query) > 0.1
		)
		ORDER BY
			CASE WHEN LOWER(users.username) LIKE LOWER('%' || @query || '%') THEN 1 ELSE 2 END,
			similarity(users.username, @query) DESC,
			followers_count DESC, 
			username
	`
	args := pgx.StrictNamedArgs{
		"query":             in.Query,
		"logged_in_user_id": in.LoggedInUserID(),
	}

	query = addLimitAndOffset(query, args, in.PageArgs)

	rows, err := c.db.Query(ctx, query, args)
	if err != nil {
		return out, fmt.Errorf("sql search users: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.User])
	if err != nil {
		return out, fmt.Errorf("sql collect searched users: %w", err)
	}

	applySimplePageInfo(&out, in.PageArgs)

	return out, nil
}
