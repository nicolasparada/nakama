package cockroach

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/go-errs"
)

func (c *Cockroach) ToggleFollow(ctx context.Context, followerID, followeeID string) (types.ToggledFollow, error) {
	var out types.ToggledFollow
	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		followExists, err := c.followExists(ctx, followerID, followeeID)
		if err != nil {
			return err
		}

		if followExists {
			if err := c.deleteFollow(ctx, followerID, followeeID); err != nil {
				return err
			}

			if _, err := c.decreaseFolloweesCount(ctx, followerID); err != nil {
				return err
			}

			followersCount, err := c.decreaseFollowersCount(ctx, followeeID)
			if err != nil {
				return err
			}

			out.FollowedByViewer = false
			out.FollowersCount = followersCount

			return nil
		}

		if err := c.createFollow(ctx, followerID, followeeID); err != nil {
			return err
		}

		if _, err := c.increaseFolloweesCount(ctx, followerID); err != nil {
			return err
		}

		followersCount, err := c.increaseFollowersCount(ctx, followeeID)
		if err != nil {
			return err
		}

		out.FollowedByViewer = true
		out.FollowersCount = followersCount

		return nil
	})
}

func (c *Cockroach) createFollow(ctx context.Context, followerID, followeeID string) error {
	query := `
		INSERT INTO follows (follower_id, followee_id) VALUES (@follower_id, @followee_id)
	`

	args := pgx.StrictNamedArgs{
		"follower_id": followerID,
		"followee_id": followeeID,
	}

	_, err := c.db.Exec(ctx, query, args)
	if db.IsUniqueViolationError(err) {
		return errs.ConflictError("follow already exists")
	}

	if err != nil {
		return fmt.Errorf("sql insert follow: %w", err)
	}

	return nil
}

func (c *Cockroach) followExists(ctx context.Context, followerID, followeeID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM follows WHERE follower_id = @follower_id AND followee_id = @followee_id
		)
	`

	args := pgx.StrictNamedArgs{
		"follower_id": followerID,
		"followee_id": followeeID,
	}

	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql select follow existence: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) deleteFollow(ctx context.Context, followerID, followeeID string) error {
	query := `
		DELETE FROM follows WHERE follower_id = @follower_id AND followee_id = @followee_id
	`

	args := pgx.StrictNamedArgs{
		"follower_id": followerID,
		"followee_id": followeeID,
	}

	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete follow: %w", err)
	}

	return nil
}
