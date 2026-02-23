package cockroach

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (c *Cockroach) IncreasePostCommentsCount(ctx context.Context, postID string) error {
	const query = "UPDATE posts SET comments_count = comments_count + 1 WHERE id = @post_id"
	_, err := c.db.Exec(ctx, query, pgx.StrictNamedArgs{"post_id": postID})
	if err != nil {
		return fmt.Errorf("could not increase post comments count: %w", err)
	}

	return nil
}
