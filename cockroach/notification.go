package cockroach

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/go-errs"
)

const notificationsCols = `
	  notifications.id
	, notifications.user_id
	, notifications.actor_user_ids
	, notifications.actors_count
	, notifications.kind
	, notifications.post_id
	, notifications.comment_id
	, notifications.read_at
	, notifications.issued_at
	, (notifications.read_at IS NOT NULL) AS read
`

const sqlSelectNotificationActors = `
	COALESCE(
		json_agg(
			json_build_object(
				  'id', actor_users.id
				, 'username', actor_users.username
				, 'avatarURL', actor_users.avatar
			) ORDER BY array_position(notifications.actor_user_ids, actor_users.id)
		) FILTER (WHERE actor_users.id IS NOT NULL),
		'[]'::json
	) AS actors
`

const sqlSelectNotificationPostPreview = `
	CASE 
		WHEN posts.id IS NOT NULL 
		THEN json_build_object(
			  'id', posts.id
			, 'userID', posts.user_id
			, 'content', posts.content
			, 'spoilerOf', posts.spoiler_of
			, 'nsfw', posts.nsfw
			, 'mediaURLs', posts.media
			, 'mine', (posts.user_id = notifications.user_id)
		)
		ELSE NULL
	END AS post
`

func (c *Cockroach) Notifications(ctx context.Context, in types.ListNotifications) (types.Page[types.Notification], error) {
	var out types.Page[types.Notification]

	args := pgx.StrictNamedArgs{"user_id": in.UserID()}
	selects := []string{
		notificationsCols,
		sqlSelectNotificationActors,
		sqlSelectNotificationPostPreview,
		sqlSelectCommentPreview,
	}
	joins := []string{
		`LEFT JOIN users actor_users ON actor_users.id = ANY(notifications.actor_user_ids)`,
		`LEFT JOIN posts ON notifications.post_id = posts.id`,
		`LEFT JOIN comments ON notifications.comment_id = comments.id`,
	}
	filters := []string{"notifications.user_id = @user_id"}

	pageArgs, err := ParsePageArgs[time.Time](in.PageArgs)
	if err != nil {
		return out, err
	}

	if pageArgs.After != nil {
		filters = append(filters, "(notifications.issued_at, notifications.id) < (@after_issued_at, @after_id)")
		args["after_issued_at"] = pageArgs.After.Value
		args["after_id"] = pageArgs.After.ID
	} else if pageArgs.Before != nil {
		filters = append(filters, "(notifications.issued_at, notifications.id) > (@before_issued_at, @before_id)")
		args["before_issued_at"] = pageArgs.Before.Value
		args["before_id"] = pageArgs.Before.ID
	}

	var order, limit string
	if pageArgs.IsBackwards() {
		order = "ORDER BY notifications.issued_at ASC, notifications.id ASC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.Last, defaultPageSize)+1) // +1 to check if there's a next page
	} else {
		order = "ORDER BY notifications.issued_at DESC, notifications.id DESC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.First, defaultPageSize)+1) // +1 to check if there's a next page
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM notifications
		%s
		WHERE %s
		GROUP BY notifications.id, posts.id, comments.id
		%s
		%s
	`, strings.Join(selects, ", "),
		strings.Join(joins, "\n"),
		strings.Join(filters, " AND "),
		order,
		limit,
	)

	notifications, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Notification])
	if err != nil {
		return out, fmt.Errorf("sql select notifications: %w", err)
	}

	out.Items = notifications

	return out, applyPageInfo(&out, pageArgs, func(n types.Notification) Cursor[time.Time] {
		return Cursor[time.Time]{ID: n.ID, Value: n.IssuedAt}
	})
}

func (c *Cockroach) Notification(ctx context.Context, notificationID string) (types.Notification, error) {
	args := pgx.StrictNamedArgs{"notification_id": notificationID}
	selects := []string{
		notificationsCols,
		sqlSelectNotificationActors,
		sqlSelectNotificationPostPreview,
		sqlSelectCommentPreview,
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM notifications
		LEFT JOIN users actor_users ON actor_users.id = ANY(notifications.actor_user_ids)
		LEFT JOIN posts ON notifications.post_id = posts.id
		LEFT JOIN comments ON notifications.comment_id = comments.id
		WHERE notifications.id = @notification_id
		GROUP BY notifications.id, posts.id, comments.id
	`, strings.Join(selects, ", "))

	notification, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Notification])
	if db.IsNotFoundError(err) {
		return notification, errs.NotFoundError("notification not found")
	}

	if err != nil {
		return notification, fmt.Errorf("sql select notification by ID: %w", err)
	}

	return notification, nil
}

func (c *Cockroach) HasUnreadNotifications(ctx context.Context, userID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM notifications
			WHERE user_id = @user_id
			  AND read_at IS NULL
		)
	`

	args := pgx.StrictNamedArgs{"user_id": userID}

	hasUnread, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql select has unread notifications: %w", err)
	}

	return hasUnread, nil
}

func (c *Cockroach) MarkNotificationAsRead(ctx context.Context, notificationID, userID string) error {
	const query = `
		UPDATE notifications
		SET read_at = now()
		WHERE id = @notification_id
		  AND user_id = @user_id
		  AND read_at IS NULL
	`

	args := pgx.StrictNamedArgs{
		"notification_id": notificationID,
		"user_id":         userID,
	}

	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql update notification and mark as read: %w", err)
	}

	return nil
}

func (c *Cockroach) MarkNotificationsAsRead(ctx context.Context, userID string) error {
	const query = `
		UPDATE notifications
		SET read_at = now()
		WHERE user_id = @user_id
		  AND read_at IS NULL
	`

	args := pgx.StrictNamedArgs{"user_id": userID}

	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql update notifications and mark as read: %w", err)
	}

	return nil
}

func (c *Cockroach) CreateFollowNotification(ctx context.Context, userID, actorUserID string) (*string, error) {
	var notificationID *string

	return notificationID, c.db.RunTx(ctx, func(ctx context.Context) error {
		actorExists, err := c.notificationActorExists(ctx, userID, actorUserID, types.NotificationKindFollow)
		if err != nil {
			return err
		}

		// prevent spamming follow notification since following is a toggle.
		if actorExists {
			return nil
		}

		notificationID, err = c.notificationIDFromUnreadFollow(ctx, userID)
		if err != nil {
			return err
		}

		// if there is no unread notification, then create a new one.
		if notificationID == nil {
			created, err := c.createNotification(ctx, types.CreateNotification{
				UserID: userID,
				Kind:   types.NotificationKindFollow,
			})
			if err != nil {
				return err
			}

			notificationID = &created.ID
		}

		return c.upsertNotificationActor(ctx, *notificationID, actorUserID)
	})
}

func (c *Cockroach) FanoutCommentNotification(ctx context.Context, in types.FanoutCommentNotification) ([]types.CreatedNotification, error) {
	var createdList []types.CreatedNotification
	return createdList, c.db.RunTx(ctx, func(ctx context.Context) error {
		var err error
		createdList, err = c.fanoutCommentNotification(ctx, in)
		if err != nil {
			return err
		}

		return c.upsertManyNotificationsActor(ctx, createdList, in.ActorUserID)
	})
}

func (c *Cockroach) NotificationsByIDs(ctx context.Context, notificationIDs []string) ([]types.Notification, error) {
	if len(notificationIDs) == 0 {
		return nil, nil
	}

	args := pgx.StrictNamedArgs{"notification_ids": notificationIDs}
	selects := []string{
		notificationsCols,
		sqlSelectNotificationActors,
		sqlSelectNotificationPostPreview,
		sqlSelectCommentPreview,
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM notifications
		LEFT JOIN users actor_users ON actor_users.id = ANY(notifications.actor_user_ids)
		LEFT JOIN posts ON notifications.post_id = posts.id
		LEFT JOIN comments ON notifications.comment_id = comments.id
		WHERE notifications.id = ANY(@notification_ids)
		GROUP BY notifications.id, posts.id, comments.id
		ORDER BY array_position(@notification_ids::UUID[], notifications.id)
	`, strings.Join(selects, ", "))

	notifications, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Notification])
	if err != nil {
		return nil, fmt.Errorf("sql select notifications by IDs: %w", err)
	}

	return notifications, nil
}

func (c *Cockroach) fanoutCommentNotification(ctx context.Context, in types.FanoutCommentNotification) ([]types.CreatedNotification, error) {
	const query = `
		INSERT INTO notifications (user_id, kind, post_id)
		SELECT post_subscriptions.user_id, @kind, post_subscriptions.post_id
		FROM post_subscriptions
		WHERE post_subscriptions.user_id != @actor_user_id
		  AND post_subscriptions.post_id = @post_id
		ON CONFLICT (user_id, kind, post_id) WHERE kind = 'comment' AND read_at IS NULL DO UPDATE SET issued_at = now()
		RETURNING id, issued_at
	`

	args := pgx.StrictNamedArgs{
		"actor_user_id": in.ActorUserID,
		"kind":          types.NotificationKindComment,
		"post_id":       in.PostID,
	}

	notifications, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.CreatedNotification])
	if err != nil {
		return nil, fmt.Errorf("sql fanout comment notifications: %w", err)
	}

	return notifications, nil
}

func (c *Cockroach) CreateMentionNotifications(ctx context.Context, in types.CreateMentionNotifications) ([]types.CreatedNotification, error) {
	var createdList []types.CreatedNotification
	return createdList, c.db.RunTx(ctx, func(ctx context.Context) error {
		var err error
		createdList, err = c.createMentionNotifications(ctx, in)
		if err != nil {
			return err
		}

		return c.upsertManyNotificationsActor(ctx, createdList, in.ActorUserID)
	})
}

func (c *Cockroach) createMentionNotifications(ctx context.Context, in types.CreateMentionNotifications) ([]types.CreatedNotification, error) {
	if len(in.Mentions) == 0 {
		return nil, nil
	}

	const query = `
		INSERT INTO notifications (user_id, kind, post_id, comment_id)
		SELECT users.id, @kind, @post_id, @comment_id
		FROM users
		WHERE users.username = ANY(@mentions) AND users.id != @actor_user_id
		RETURNING id, issued_at
	`

	args := pgx.StrictNamedArgs{
		"actor_user_id": in.ActorUserID,
		"kind":          in.Kind,
		"post_id":       in.PostID,
		"comment_id":    in.CommentID,
		"mentions":      in.Mentions,
	}

	createdList, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.CreatedNotification])
	if err != nil {
		return nil, fmt.Errorf("sql create %q mention notifications: %w", in.Kind, err)
	}

	return createdList, nil
}

func (c *Cockroach) createNotification(ctx context.Context, in types.CreateNotification) (types.CreatedNotification, error) {
	const query = `
		INSERT INTO notifications (user_id, kind, post_id, comment_id)
		VALUES (@user_id, @kind, @post_id, @comment_id)
		RETURNING id, issued_at
	`

	args := pgx.StrictNamedArgs{
		"user_id":    in.UserID,
		"kind":       in.Kind,
		"post_id":    in.PostID,
		"comment_id": in.CommentID,
	}

	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.CreatedNotification])
	if err != nil {
		return out, fmt.Errorf("sql insert notification: %w", err)
	}

	return out, nil
}

func (c *Cockroach) notificationIDFromUnreadFollow(ctx context.Context, userID string) (*string, error) {
	const query = `
		SELECT id
		FROM notifications
		WHERE user_id = @user_id
		  AND kind = @kind
		  AND read_at IS NULL
	`

	args := pgx.StrictNamedArgs{
		"user_id": userID,
		"kind":    types.NotificationKindFollow,
	}

	notificationID, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if db.IsNotFoundError(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("sql select unread follow notification ID: %w", err)
	}

	return &notificationID, nil
}

func (c *Cockroach) notificationActorExists(ctx context.Context, userID, actorUserID string, kind types.NotificationKind) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM notification_actors
			INNER JOIN notifications ON notification_actors.notification_id = notifications.id
			WHERE notifications.user_id = @user_id AND notifications.kind = @kind AND notification_actors.user_id = @actor_user_id
		)
	`

	row := c.db.QueryRow(ctx, q, pgx.StrictNamedArgs{
		"user_id":       userID,
		"kind":          kind,
		"actor_user_id": actorUserID,
	})

	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, fmt.Errorf("sql check follow notification by actor exists: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) upsertNotificationActor(ctx context.Context, notificationID, actorUserID string) error {
	const query = `
		INSERT INTO notification_actors (user_id, notification_id)
		VALUES (@user_id, @notification_id)
		ON CONFLICT (user_id, notification_id) DO NOTHING
	`

	args := pgx.StrictNamedArgs{
		"user_id":         actorUserID,
		"notification_id": notificationID,
	}

	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql upsert notification actor: %w", err)
	}

	return nil
}
func (c *Cockroach) upsertManyNotificationsActor(ctx context.Context, notifications []types.CreatedNotification, actorUserID string) error {
	if len(notifications) == 0 {
		return nil
	}

	var values []string
	args := pgx.StrictNamedArgs{}
	for i, notification := range notifications {
		userIDArg := fmt.Sprintf("user_id_%d", i)
		notificationIDArg := fmt.Sprintf("notification_id_%d", i)

		values = append(values, fmt.Sprintf("(@%s, @%s)", userIDArg, notificationIDArg))
		args[userIDArg] = actorUserID
		args[notificationIDArg] = notification.ID
	}

	query := fmt.Sprintf(`
		INSERT INTO notification_actors (user_id, notification_id)
		VALUES %s
		ON CONFLICT (user_id, notification_id) DO NOTHING
	`, strings.Join(values, ", "))

	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql upsert many notifications actor: %w", err)
	}

	return nil
}
