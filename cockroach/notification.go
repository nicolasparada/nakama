package cockroach

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-db"
)

const notificationsCols = `
	  notifications.id
	, notifications.user_id
	, notifications.actor_usernames
	, notifications.kind
	, notifications.post_id
	, notifications.comment_id
	, notifications.read_at
	, notifications.issued_at
	, (notifications.read_at IS NOT NULL AND notifications.read_at != '0001-01-01 00:00:00') AS read
`

func (c *Cockroach) Notifications(ctx context.Context, in types.ListNotifications) (types.Page[types.Notification], error) {
	var out types.Page[types.Notification]

	args := pgx.StrictNamedArgs{"user_id": in.UserID()}
	selects := []string{
		notificationsCols,
		sqlSelectPostPreview(args, in.UserID()),
		sqlSelectCommentPreview,
	}
	joins := []string{
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

func (c *Cockroach) HasUnreadNotifications(ctx context.Context, userID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM notifications
			WHERE user_id = @user_id
			  AND (read_at IS NULL OR read_at = '0001-01-01 00:00:00')
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
		  AND (read_at IS NULL OR read_at = '0001-01-01 00:00:00')
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
		  AND (read_at IS NULL OR read_at = '0001-01-01 00:00:00')
	`

	args := pgx.StrictNamedArgs{"user_id": userID}

	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql update notifications and mark as read: %w", err)
	}

	return nil
}

func (c *Cockroach) CreateFollowNotification(ctx context.Context, userID, actorID string) (*types.Notification, error) {
	var out *types.Notification

	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		actorUsername, err := c.usernameFromUserID(ctx, actorID)
		if err != nil {
			return err
		}

		existsReadOrNot, err := c.notificationExists(ctx, types.NotificationExists{
			UserID:        &userID,
			ActorUsername: &actorUsername,
			Kind:          new(types.NotificationKindFollow),
		})
		if err != nil {
			return err
		}

		// prevent spamming follow notification since following is a toggle.
		if existsReadOrNot {
			return nil
		}

		// groupping follow notifications by reusing the same notification and prepending new actors to the actor_usernames array.
		notificationID, err := c.notificationIDFromUnreadFollow(ctx, userID)
		if err != nil {
			return err
		}

		if notificationID != nil {
			addedActor, err := c.addNotificationActor(ctx, *notificationID, actorUsername)
			if err != nil {
				return err
			}

			out = &types.Notification{
				ID:             *notificationID,
				UserID:         userID,
				ActorUsernames: addedActor.ActorUsernames,
				Kind:           types.NotificationKindFollow,
				IssuedAt:       addedActor.IssuedAt,
			}

			return nil
		}

		created, err := c.createNotification(ctx, types.CreateNotification{
			UserID:         userID,
			ActorUsernames: []string{actorUsername},
			Kind:           types.NotificationKindFollow,
		})
		if err != nil {
			return err
		}

		out = &types.Notification{
			ID:             created.ID,
			UserID:         userID,
			ActorUsernames: []string{actorUsername},
			Kind:           types.NotificationKindFollow,
			IssuedAt:       created.IssuedAt,
		}

		return nil
	})
}

func (c *Cockroach) FanoutCommentNotification(ctx context.Context, in types.FanoutCommentNotification) ([]types.Notification, error) {
	query := fmt.Sprintf(`
		INSERT INTO notifications (user_id, actor_usernames, kind, post_id, comment_id, read_at)
		SELECT post_subscriptions.user_id, @actor_usernames, @kind, @post_id, @comment_id, '0001-01-01 00:00:00'
		FROM post_subscriptions
		WHERE post_subscriptions.user_id != @actor_user_id
		  AND post_subscriptions.post_id = @post_id
		ON CONFLICT (user_id, kind, post_id, comment_id, read_at) DO UPDATE SET
			actor_usernames = array_prepend(@actor_username, array_remove(notifications.actor_usernames, @actor_username)),
			issued_at = now()
		RETURNING %s
	`, notificationsCols)

	args := pgx.StrictNamedArgs{
		"actor_usernames": []string{in.ActorUsername},
		"actor_username":  in.ActorUsername,
		"actor_user_id":   in.ActorUserID,
		"kind":            types.NotificationKindComment,
		"post_id":         in.PostID,
		"comment_id":      in.CommentID,
	}

	notifications, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Notification])
	if err != nil {
		return nil, fmt.Errorf("sql fanout comment notifications: %w", err)
	}

	return notifications, nil
}

func (c *Cockroach) CreateMentionNotifications(ctx context.Context, in types.CreateMentionNotifications) ([]types.Notification, error) {
	if len(in.Mentions) == 0 {
		return nil, nil
	}

	query := fmt.Sprintf(`
		INSERT INTO notifications (user_id, actor_usernames, kind, post_id, comment_id, read_at)
		SELECT users.id, @actor_usernames, @kind, @post_id, @comment_id, '0001-01-01 00:00:00'
		FROM users
		WHERE users.username = ANY(@mentions)
		  AND users.id != @actor_user_id
		ON CONFLICT (user_id, kind, post_id, comment_id, read_at) DO UPDATE SET
			actor_usernames = array_prepend(@actor_username, array_remove(notifications.actor_usernames, @actor_username)),
			issued_at = now()
		RETURNING %s
	`, notificationsCols)

	args := pgx.StrictNamedArgs{
		"actor_usernames": []string{in.ActorUsername},
		"actor_username":  in.ActorUsername,
		"actor_user_id":   in.ActorUserID,
		"kind":            in.Kind,
		"post_id":         in.PostID,
		"comment_id":      in.CommentID,
		"mentions":        in.Mentions,
	}

	notifications, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Notification])
	if err != nil {
		return nil, fmt.Errorf("sql create %q mention notifications: %w", in.Kind, err)
	}

	return notifications, nil
}

func (c *Cockroach) addNotificationActor(ctx context.Context, notificationID, actorUsername string) (types.AddedNotificationActor, error) {
	const query = `
		UPDATE notifications
		SET
			actor_usernames = array_prepend(@actor_username, notifications.actor_usernames),
			issued_at = now()
		WHERE id = @notification_id
		RETURNING actor_usernames, issued_at
	`

	args := pgx.StrictNamedArgs{
		"actor_username":  actorUsername,
		"notification_id": notificationID,
	}

	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.AddedNotificationActor])
	if err != nil {
		return out, fmt.Errorf("sql update notification and add actor: %w", err)
	}

	return out, nil
}

func (c *Cockroach) createNotification(ctx context.Context, in types.CreateNotification) (types.CreatedNotification, error) {
	const query = `
		INSERT INTO notifications (user_id, actor_usernames, kind, post_id)
		VALUES (@user_id, @actor_usernames, @kind, @post_id)
		RETURNING id, issued_at
	`

	args := pgx.StrictNamedArgs{
		"user_id":         in.UserID,
		"actor_usernames": in.ActorUsernames,
		"kind":            in.Kind,
		"post_id":         in.PostID,
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
		  AND kind = 'follow'
		  AND (read_at IS NULL OR read_at = '0001-01-01 00:00:00')
	`

	args := pgx.StrictNamedArgs{"user_id": userID}

	notificationID, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if db.IsNotFoundError(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("sql select unread follow notification ID: %w", err)
	}

	return &notificationID, nil
}

func (c *Cockroach) notificationExists(ctx context.Context, in types.NotificationExists) (bool, error) {
	if in.IsEmpty() {
		return false, errors.New("notification existence check requires at least one field to be set")
	}

	args := pgx.StrictNamedArgs{}
	filters := []string{}

	if in.UserID != nil {
		filters = append(filters, "user_id = @user_id")
		args["user_id"] = *in.UserID
	}

	if in.ActorUsername != nil {
		filters = append(filters, "@actor_username::varchar = ANY(actor_usernames)")
		args["actor_username"] = *in.ActorUsername
	}

	if in.Kind != nil {
		filters = append(filters, "kind = @kind")
		args["kind"] = in.Kind.String()
	}

	if in.PostID != nil {
		filters = append(filters, "post_id = @post_id")
		args["post_id"] = *in.PostID
	}

	query := fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1
			FROM notifications
			WHERE %s
		)
	`, strings.Join(filters, " AND "))

	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql select notification existence: %w", err)
	}

	return exists, nil
}
