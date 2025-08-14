package cockroach

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

func (c *Cockroach) Notifications(ctx context.Context, in types.ListNotifications) (types.Page[types.Notification], error) {
	var out types.Page[types.Notification]

	const q = `
		SELECT 
			notifications.*,
			COALESCE(
				json_agg(
					json_build_object(
						'id', users.id,
						'username', users.username
					) ORDER BY array_position(notifications.actor_user_ids, users.id)
				) FILTER (WHERE users.id IS NOT NULL),
				'[]'::json
			) AS actors
		FROM notifications
		LEFT JOIN users ON users.id = ANY(notifications.actor_user_ids)
		WHERE notifications.user_id = @user_id
		GROUP BY notifications.id
		ORDER BY notifications.id DESC
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"user_id": in.UserID(),
	})
	if err != nil {
		return out, fmt.Errorf("sql select notifications: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Notification])
	if err != nil {
		return out, fmt.Errorf("sql collect notifications: %w", err)
	}

	return out, nil
}

func (c *Cockroach) ReadNotification(ctx context.Context, in types.ReadNotification) error {
	const q = `
		UPDATE notifications
		SET read_at = NOW()
		WHERE id = @notification_id AND user_id = @user_id
	`

	_, err := c.db.Exec(ctx, q, pgx.StrictNamedArgs{
		"notification_id": in.NotificationID,
		"user_id":         in.UserID(),
	})
	if err != nil {
		return fmt.Errorf("sql update notification read_at: %w", err)
	}

	return nil
}

func (c *Cockroach) CreateFollowNotification(ctx context.Context, in types.CreateFollowNotification) error {
	return c.db.RunTx(ctx, func(ctx context.Context) error {
		actorExists, err := c.followNotificationActorExists(ctx, in)
		if err != nil {
			return err
		}

		if actorExists {
			return nil
		}

		notificationID, hasUnread, err := c.unreadFollowNotificationID(ctx, in.UserID)
		if err != nil {
			return err
		}

		if !hasUnread {
			created, err := c.createFollowNotification(ctx, in)
			if err != nil {
				return err
			}

			notificationID = created.ID
		}

		return c.upsertNotificationActor(ctx, notificationID, in.ActorUserID)
	})
}

func (c *Cockroach) CreateMentionNotifications(ctx context.Context, in types.CreateMentionNotifications) error {
	return c.db.RunTx(ctx, func(ctx context.Context) error {
		createdList, err := c.createMentionNotifications(ctx, in)
		if err != nil {
			return err
		}

		return c.upsertManyNotificationsActor(ctx, createdList, in.ActorUserID)
	})
}

func (c *Cockroach) createMentionNotifications(ctx context.Context, in types.CreateMentionNotifications) ([]types.Created, error) {
	var out []types.Created

	const q = `
		INSERT INTO notifications (id, user_id, kind, notifiable_kind, notifiable_id, actor_user_ids)
		SELECT
			uuid_to_ulid(gen_random_ulid()),
			users.id,
			@kind,
			@notifiable_kind,
			@notifiable_id,
			@actor_user_ids
		FROM users
		WHERE users.username = ANY(@usernames)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"kind":            types.NotificationKindPostMention,
		"notifiable_kind": in.NotifiableKind,
		"notifiable_id":   in.NotifiableID,
		// This should be filled by a trigger,
		// but this avoids the immediate NOT NULL constraint
		// on actor_user_ids
		"actor_user_ids": []string{in.ActorUserID},
		"usernames":      in.Usernames,
	})
	if err != nil {
		return out, fmt.Errorf("sql insert mention notifications: %w", err)
	}

	out, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted mention notifications: %w", err)
	}

	return out, nil
}

func (c *Cockroach) followNotificationActorExists(ctx context.Context, in types.CreateFollowNotification) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM notification_actors
			INNER JOIN notifications ON notification_actors.notification_id = notifications.id
			WHERE notifications.user_id = @user_id AND notifications.kind = @kind AND notification_actors.user_id = @actor_user_id
		)
	`

	row := c.db.QueryRow(ctx, q, pgx.StrictNamedArgs{
		"user_id":       in.UserID,
		"kind":          types.NotificationKindFollow,
		"actor_user_id": in.ActorUserID,
	})

	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, fmt.Errorf("sql check follow notification by actor exists: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) unreadFollowNotificationID(ctx context.Context, userID string) (string, bool, error) {
	var out string

	const q = `
		SELECT id FROM notifications
		WHERE user_id = @user_id AND kind = @kind AND read_at IS NULL
		LIMIT 1
	`

	row := c.db.QueryRow(ctx, q, pgx.StrictNamedArgs{
		"user_id": userID,
		"kind":    types.NotificationKindFollow,
	})

	err := row.Scan(&out)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}

	if err != nil {
		return "", false, fmt.Errorf("sql query unread follow notification id: %w", err)
	}

	return out, true, nil
}

func (c *Cockroach) createFollowNotification(ctx context.Context, in types.CreateFollowNotification) (types.Created, error) {
	var out types.Created

	const q = `
		INSERT INTO notifications (id, user_id, kind, actor_user_ids)
		VALUES (@notification_id, @user_id, @kind, @actor_user_ids)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"notification_id": id.Generate(),
		"user_id":         in.UserID,
		"kind":            types.NotificationKindFollow,
		// This should be filled by a trigger,
		// but this avoids the immediate NOT NULL constraint
		// on actor_user_ids
		"actor_user_ids": []string{in.ActorUserID},
	})
	if err != nil {
		return out, fmt.Errorf("sql insert follow notification: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted follow notification: %w", err)
	}

	return out, nil
}

func (c *Cockroach) upsertNotificationActor(ctx context.Context, notificationID, actorUserID string) error {
	const q = `
		INSERT INTO notification_actors (user_id, notification_id)
		VALUES (@user_id, @notification_id)
		ON CONFLICT (user_id, notification_id) DO UPDATE SET updated_at = NOW()
	`

	_, err := c.db.Exec(ctx, q, pgx.StrictNamedArgs{
		"user_id":         actorUserID,
		"notification_id": notificationID,
	})
	if err != nil {
		return fmt.Errorf("sql upsert notification actor: %w", err)
	}

	return nil
}
func (c *Cockroach) upsertManyNotificationsActor(ctx context.Context, notifications []types.Created, actorUserID string) error {
	if len(notifications) == 0 {
		return nil
	}

	var values []string
	args := pgx.StrictNamedArgs{}
	for i, notification := range notifications {
		values = append(values, fmt.Sprintf("(@user_id_%d, @notification_id_%d)", i, i))
		args[fmt.Sprintf("user_id_%d", i)] = actorUserID
		args[fmt.Sprintf("notification_id_%d", i)] = notification.ID
	}

	query := fmt.Sprintf(`
		INSERT INTO notification_actors (user_id, notification_id)
		VALUES %s
		ON CONFLICT (user_id, notification_id) DO UPDATE SET updated_at = NOW()
	`, strings.Join(values, ", "))

	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql upsert many notifications actor: %w", err)
	}

	return nil
}
