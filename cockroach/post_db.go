package cockroach

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

func (c *Cockroach) CreatePost(ctx context.Context, in types.CreatePost) (types.Created, error) {
	var out types.Created
	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = c.createPost(ctx, in)
		if err != nil {
			return err
		}

		return c.createFeedItem(ctx, types.FanoutPost{
			UserID: in.UserID(),
			PostID: out.ID,
		})
	})
}

func (c *Cockroach) createPost(ctx context.Context, in types.CreatePost) (types.Created, error) {
	var out types.Created

	const q = `
		INSERT INTO posts (id, user_id, content, is_r18, attachments)
		VALUES (@post_id, @user_id, @content, @is_r18, @attachments)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"post_id":     id.Generate(),
		"user_id":     in.UserID(),
		"content":     in.Content,
		"is_r18":      in.IsR18,
		"attachments": in.ProcessedAttachments(),
	})
	if err != nil {
		return out, fmt.Errorf("sql insert post: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted post: %w", err)
	}

	return out, nil
}

func (c *Cockroach) createFeedItem(ctx context.Context, in types.FanoutPost) error {
	const q = `
		INSERT INTO feed (user_id, post_id)
		VALUES (@user_id, @post_id)
	`

	_, err := c.db.Exec(ctx, q, pgx.NamedArgs{
		"user_id": in.UserID,
		"post_id": in.PostID,
	})
	if err != nil {
		return fmt.Errorf("sql insert feed: %w", err)
	}

	return nil
}

func (c *Cockroach) Feed(ctx context.Context, in types.ListFeed) (types.Page[types.Post], error) {
	var out types.Page[types.Post]

	const q = `
		SELECT
			posts.*,
			to_json(users) AS user
		FROM feed
		INNER JOIN posts ON feed.post_id = posts.id
		INNER JOIN users ON posts.user_id = users.id
		WHERE feed.user_id = @user_id
		ORDER BY posts.id DESC
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"user_id": in.UserID(),
	})
	if err != nil {
		return out, fmt.Errorf("sql select feed: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Post])
	if err != nil {
		return out, fmt.Errorf("sql collect feed: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Posts(ctx context.Context) (types.Page[types.Post], error) {
	var out types.Page[types.Post]

	const q = `
		SELECT 
			posts.*,
			to_json(users) AS user
		FROM posts
		INNER JOIN users ON posts.user_id = users.id
		ORDER BY posts.id DESC
	`

	rows, err := c.db.Query(ctx, q)
	if err != nil {
		return out, fmt.Errorf("sql select posts: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Post])
	if err != nil {
		return out, fmt.Errorf("sql collect posts: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Post(ctx context.Context, postID string) (types.Post, error) {
	var out types.Post

	const q = `
		SELECT posts.*, to_json(users) AS user
		FROM posts
		INNER JOIN users ON posts.user_id = users.id
		WHERE posts.id = @post_id
	`

	rows, err := c.db.Query(ctx, q, pgx.NamedArgs{
		"post_id": postID,
	})
	if err != nil {
		return out, fmt.Errorf("sql select post: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Post])
	if err != nil {
		return out, fmt.Errorf("sql collect post: %w", err)
	}

	return out, nil
}

func (c *Cockroach) FanoutPost(ctx context.Context, in types.FanoutPost) error {
	const q = `
		INSERT INTO feed (user_id, post_id)
		SELECT follows.follower_id, @post_id
		FROM follows
		WHERE follows.followee_id = @user_id
	`

	_, err := c.db.Exec(ctx, q, pgx.NamedArgs{
		"user_id": in.UserID,
		"post_id": in.PostID,
	})
	if err != nil {
		return fmt.Errorf("sql fanout feed: %w", err)
	}

	return nil
}
