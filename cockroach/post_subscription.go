package cockroach

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (c *Cockroach) UpsertPostSubscription(ctx context.Context, userID, postID string) error {
	const query = `
		INSERT INTO post_subscriptions (user_id, post_id) VALUES (@user_id, @post_id)
		ON CONFLICT (user_id, post_id) DO NOTHING
	`
	args := pgx.StrictNamedArgs{
		"user_id": userID,
		"post_id": postID,
	}
	if _, err := c.db.Exec(ctx, query, args); err != nil {
		return fmt.Errorf("sql upsert post subscription: %w", err)
	}

	return nil
}
