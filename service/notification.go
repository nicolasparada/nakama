package service

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nakamauwu/nakama/cockroach"
	"github.com/nakamauwu/nakama/cursor"
	"github.com/nakamauwu/nakama/textutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

// ErrInvalidNotificationID denotes an invalid notification id; that is not uuid.
var ErrInvalidNotificationID = errs.InvalidArgumentError("invalid notification ID")

// Notifications from the authenticated user in descending order with backward pagination.
func (s *Service) Notifications(ctx context.Context, last uint64, before *string) (types.Notifications, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, errs.Unauthenticated
	}

	var beforeNotificationID string
	var beforeIssuedAt time.Time

	if before != nil {
		var err error
		beforeNotificationID, beforeIssuedAt, err = cursor.Decode(*before)
		if err != nil || !types.ValidUUIDv4(beforeNotificationID) {
			return nil, ErrInvalidCursor
		}
	}

	last = normalizePageSize(last)
	query, args, err := buildQuery(`
		SELECT id
		, actors
		, type
		, post_id
		, read_at
		, issued_at
		FROM notifications
		WHERE user_id = @uid
		{{ if and .beforeNotificationID .beforeIssuedAt }}
			AND issued_at <= @beforeIssuedAt
			AND (
				id < @beforeNotificationID
					OR issued_at < @beforeIssuedAt
			)
		{{ end }}
		ORDER BY issued_at DESC
		LIMIT @last`, map[string]any{
		"uid":                  uid,
		"last":                 last,
		"beforeNotificationID": beforeNotificationID,
		"beforeIssuedAt":       beforeIssuedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build notifications sql query: %w", err)
	}

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select notifications: %w", err)
	}

	defer rows.Close()

	var nn types.Notifications
	for rows.Next() {
		var n types.Notification
		var readAt *time.Time
		if err = rows.Scan(&n.ID, &n.Actors, &n.Type, &n.PostID, &readAt, &n.IssuedAt); err != nil {
			return nil, fmt.Errorf("could not scan notification: %w", err)
		}

		n.Read = readAt != nil && !readAt.IsZero()
		nn = append(nn, n)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not iterate over notification rows: %w", err)
	}

	return nn, nil
}

// NotificationStream to receive notifications in realtime.
func (s *Service) NotificationStream(ctx context.Context) (<-chan types.Notification, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, errs.Unauthenticated
	}

	nn := make(chan types.Notification)
	unsub, err := s.PubSub.Sub(notificationTopic(uid), func(data []byte) {
		go func(r io.Reader) {
			var n types.Notification
			err := gob.NewDecoder(r).Decode(&n)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not gob decode notification: %w", err))
				return
			}

			nn <- n
		}(bytes.NewReader(data))
	})
	if err != nil {
		return nil, fmt.Errorf("could not subcribe to notifications: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := unsub(); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not unsubcribe from notifications: %w", err))
			// don't return
		}
		close(nn)
	}()

	return nn, nil
}

// HasUnreadNotifications checks if the authenticated user has any unread notification.
func (s *Service) HasUnreadNotifications(ctx context.Context) (bool, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return false, errs.Unauthenticated
	}

	var unread bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM notifications WHERE user_id = $1 AND (read_at IS NULL OR read_at = '0001-01-01 00:00:00')
	)`, uid).Scan(&unread); err != nil {
		return false, fmt.Errorf("could not query select unread notifications existence: %w", err)
	}

	return unread, nil
}

// MarkNotificationAsRead sets a notification from the authenticated user as read.
func (s *Service) MarkNotificationAsRead(ctx context.Context, notificationID string) error {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return errs.Unauthenticated
	}

	if !types.ValidUUIDv4(notificationID) {
		return ErrInvalidNotificationID
	}

	if _, err := s.DB.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE id = $1 AND user_id = $2 AND (read_at IS NULL OR read_at = '0001-01-01 00:00:00')`, notificationID, uid); err != nil {
		return fmt.Errorf("could not update and mark notification as read: %w", err)
	}

	return nil
}

// MarkNotificationsAsRead sets all notification from the authenticated user as read.
func (s *Service) MarkNotificationsAsRead(ctx context.Context) error {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return errs.Unauthenticated
	}

	if _, err := s.DB.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE user_id = $1 AND (read_at IS NULL OR read_at = '0001-01-01 00:00:00')
	`, uid); err != nil {
		return fmt.Errorf("could not update and mark notifications as read: %w", err)
	}

	return nil
}

func (s *Service) notifyFollow(followerID, followeeID string) {
	ctx := context.Background()
	var n types.Notification
	var notified bool

	err := cockroach.ExecuteTx(ctx, s.DB, func(tx pgx.Tx) error {
		var actor string
		query := "SELECT username FROM users WHERE id = $1"
		err := tx.QueryRow(ctx, query, followerID).Scan(&actor)
		if err != nil {
			return fmt.Errorf("could not query select follow notification actor: %w", err)
		}

		query = `SELECT EXISTS (
			SELECT 1 FROM notifications
			WHERE user_id = $1
				AND $2:::VARCHAR = ANY(actors)
				AND type = 'follow'
		)`
		err = tx.QueryRow(ctx, query, followeeID, actor).Scan(&notified)
		if err != nil {
			return fmt.Errorf("could not query select follow notification existence: %w", err)
		}

		if notified {
			return nil
		}

		var nid string
		query = "SELECT id FROM notifications WHERE user_id = $1 AND type = 'follow' AND (read_at IS NULL OR read_at = '0001-01-01 00:00:00')"
		err = tx.QueryRow(ctx, query, followeeID).Scan(&nid)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("could not query select unread follow notification: %w", err)
		}

		if errors.Is(err, pgx.ErrNoRows) {
			actors := []string{actor}
			query = `
				INSERT INTO notifications (user_id, actors, type) VALUES ($1, $2, 'follow')
				RETURNING id, issued_at`
			row := tx.QueryRow(ctx, query, followeeID, actors)
			err = row.Scan(&n.ID, &n.IssuedAt)
			if err != nil {
				return fmt.Errorf("could not insert follow notification: %w", err)
			}

			n.Actors = actors
		} else {
			query = `
				UPDATE notifications SET
					actors = array_prepend($1, notifications.actors),
					issued_at = now()
				WHERE id = $2
				RETURNING actors, issued_at`
			row := tx.QueryRow(ctx, query, actor, nid)
			err = row.Scan(&n.Actors, &n.IssuedAt)
			if err != nil {
				return fmt.Errorf("could not update follow notification: %w", err)
			}

			n.ID = nid
		}

		n.UserID = followeeID
		n.Type = "follow"

		return nil
	})
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not notify follow: %w", err))
		return
	}

	if !notified {
		go s.broadcastNotification(n)
	}
}

func (s *Service) notifyComment(c types.Comment) {
	actor := c.User.Username
	rows, err := s.DB.Query(context.Background(), `
		INSERT INTO notifications (user_id, actors, type, post_id, read_at)
		SELECT user_id, $1, 'comment', $2, '0001-01-01 00:00:00' FROM post_subscriptions
		WHERE post_subscriptions.user_id != $3
			AND post_subscriptions.post_id = $2
		ON CONFLICT (user_id, type, post_id, read_at) DO UPDATE SET
			actors = array_prepend($4, array_remove(notifications.actors, $4)),
			issued_at = now()
		RETURNING id, user_id, actors, issued_at`,
		[]string{actor},
		c.PostID,
		c.UserID,
		actor,
	)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not insert comment notifications: %w", err))
		return
	}

	defer rows.Close()

	for rows.Next() {
		var n types.Notification
		if err = rows.Scan(&n.ID, &n.UserID, &n.Actors, &n.IssuedAt); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not scan comment notification: %w", err))
			return
		}

		n.Type = "comment"
		n.PostID = &c.PostID

		go s.broadcastNotification(n)
	}

	if err = rows.Err(); err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not iterate over comment notification rows: %w", err))
		return
	}
}

func (s *Service) notifyPostMention(p types.Post) {
	mentions := textutil.CollectMentions(p.Content)
	if len(mentions) == 0 {
		return
	}

	actors := []string{p.User.Username}
	rows, err := s.DB.Query(context.Background(), `
		INSERT INTO notifications (user_id, actors, type, post_id)
		SELECT users.id, $1, 'post_mention', $2 FROM users
		WHERE users.id != $3
			AND username = ANY($4)
		RETURNING id, user_id, issued_at`,
		actors,
		p.ID,
		p.UserID,
		mentions,
	)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not insert post mention notifications: %w", err))
		return
	}

	defer rows.Close()

	for rows.Next() {
		var n types.Notification
		if err = rows.Scan(&n.ID, &n.UserID, &n.IssuedAt); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not scan post mention notification: %w", err))
			return
		}

		n.Actors = actors
		n.Type = "post_mention"
		n.PostID = &p.ID

		go s.broadcastNotification(n)
	}

	if err = rows.Err(); err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not iterate post mention notification rows: %w", err))
		return
	}
}

func (s *Service) notifyCommentMention(c types.Comment) {
	mentions := textutil.CollectMentions(c.Content)
	if len(mentions) == 0 {
		return
	}

	actor := c.User.Username
	rows, err := s.DB.Query(context.Background(), `
		INSERT INTO notifications (user_id, actors, type, post_id, read_at)
		SELECT users.id, $1, 'comment_mention', $2, '0001-01-01 00:00:00' FROM users
		WHERE users.id != $3
			AND username = ANY($4)
		ON CONFLICT (user_id, type, post_id, read_at) DO UPDATE SET
			actors = array_prepend($5, array_remove(notifications.actors, $5)),
			issued_at = now()
		RETURNING id, user_id, actors, issued_at`,
		[]string{actor},
		c.PostID,
		c.UserID,
		mentions,
		actor,
	)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not insert comment mention notifications: %w", err))
		return
	}

	defer rows.Close()

	for rows.Next() {
		var n types.Notification
		if err = rows.Scan(&n.ID, &n.UserID, &n.Actors, &n.IssuedAt); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not scan comment mention notification: %w", err))
			return
		}

		n.Type = "comment_mention"
		n.PostID = &c.PostID

		go s.broadcastNotification(n)
	}

	if err = rows.Err(); err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not iterate comment mention notification rows: %w", err))
		return
	}
}

func (s *Service) broadcastNotification(n types.Notification) {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(n)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not gob encode notification: %w", err))
		return
	}

	err = s.PubSub.Pub(notificationTopic(n.UserID), b.Bytes())
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not publish notification: %w", err))
		return
	}

	go s.sendWebPushNotifications(n)
}

func notificationTopic(userID string) string { return "notification_" + userID }
