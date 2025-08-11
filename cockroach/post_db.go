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
