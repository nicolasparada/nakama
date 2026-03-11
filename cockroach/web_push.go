package cockroach

import (
	"context"
	"fmt"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
)

func (c *Cockroach) UpsertWebPushSubscription(ctx context.Context, userID string, sub webpush.Subscription) error {
	const query = `
		INSERT INTO user_web_push_subscriptions (user_id, sub) VALUES (@user_id, @sub)
		ON CONFLICT (user_id, sub ->> 'endpoint') DO UPDATE SET sub = EXCLUDED.sub
	`

	args := pgx.StrictNamedArgs{
		"user_id": userID,
		"sub":     sub,
	}

	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql insert user web push subscription: %w", err)
	}

	return nil
}

func (c *Cockroach) WebPushSubscriptions(ctx context.Context, userID string) ([]webpush.Subscription, error) {
	const query = `
		SELECT sub FROM user_web_push_subscriptions WHERE user_id = @user_id ORDER BY created_at DESC
	`

	args := pgx.StrictNamedArgs{
		"user_id": userID,
	}

	subs, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowTo[webpush.Subscription])
	if err != nil {
		return nil, fmt.Errorf("sql select user web push susbcriptions: %w", err)
	}

	return subs, nil
}

func (c *Cockroach) DeleteWebPushSubscription(ctx context.Context, userID string, endpoint string) error {
	const query = `
		DELETE FROM user_web_push_subscriptions WHERE user_id = @user_id AND sub ->> 'endpoint' = @endpoint
	`

	args := pgx.StrictNamedArgs{
		"user_id":  userID,
		"endpoint": endpoint,
	}

	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete user web push subscription: %w", err)
	}

	return nil
}
