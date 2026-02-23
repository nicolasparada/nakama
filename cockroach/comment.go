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

func (c *Cockroach) CreateComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		created, err := c.createComment(ctx, in)
		if err != nil {
			return err
		}

		if err := c.UpsertPostSubscription(ctx, in.UserID(), in.PostID); err != nil {
			return err
		}

		if err := c.CreatePostTags(ctx, types.CreatePostTags{
			PostID:    in.PostID,
			CommentID: &created.ID,
			Tags:      in.Tags(),
		}); err != nil {
			return err
		}

		if err := c.IncreasePostCommentsCount(ctx, in.PostID); err != nil {
			return err
		}

		out = created

		return nil
	})
}

func (c *Cockroach) createComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	const query = `
		INSERT INTO comments (user_id, post_id, content) VALUES (@user_id, @post_id, @content)
		RETURNING id, created_at
	`
	args := pgx.StrictNamedArgs{
		"user_id": in.UserID(),
		"post_id": in.PostID,
		"content": in.Content,
	}
	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Created])
	if db.IsForeignKeyViolationError(err) {
		return out, errs.NotFoundError("post not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql insert comment: %w", err)
	}

	return out, nil
}
