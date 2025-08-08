package cockroach

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

func (c *Cockroach) CreateComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	const q = `
		INSERT INTO comments (id, user_id, post_id, content)
		VALUES (@comment_id, @user_id, @post_id, @content)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"comment_id": id.Generate(),
		"user_id":    in.UserID(),
		"post_id":    in.PostID,
		"content":    in.Content,
	})
	if db.IsForeignKeyViolationError(err, "post_id") {
		return out, errs.NewNotFoundError("post not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql insert comment: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted comment: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Comments(ctx context.Context, postID string) (types.Page[types.Comment], error) {
	var out types.Page[types.Comment]

	const q = `
		SELECT comments.*, to_json(users) AS user
		FROM comments
		INNER JOIN users ON comments.user_id = users.id
		WHERE comments.post_id = @post_id
		ORDER BY comments.id DESC
	`

	rows, err := c.db.Query(ctx, q, pgx.NamedArgs{"post_id": postID})
	if err != nil {
		return out, fmt.Errorf("sql select comments: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Comment])
	if err != nil {
		return out, fmt.Errorf("sql collect comments: %w", err)
	}

	return out, nil
}
