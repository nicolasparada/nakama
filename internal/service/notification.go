package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/lib/pq"
	"github.com/nicolasparada/nakama/internal/service/pb"
)

// Notification model.
type Notification struct {
	ID       int64     `json:"id"`
	UserID   int64     `json:"-"`
	Actors   []string  `json:"actors"`
	Type     string    `json:"type"`
	PostID   *int64    `json:"postID,omitempty"`
	Read     bool      `json:"read"`
	IssuedAt time.Time `json:"issuedAt"`
}

// Notifications from the authenticated user in descending order with backward pagination.
func (s *Service) Notifications(ctx context.Context, last int, before int64) ([]Notification, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(int64)
	if !ok {
		return nil, ErrUnauthenticated
	}

	last = normalizePageSize(last)
	query, args, err := buildQuery(`
		SELECT id, actors, type, post_id, read, issued_at
		FROM notifications
		WHERE user_id = @uid
		{{if gt .before 0}}AND id < @before{{end}}
		ORDER BY issued_at DESC
		LIMIT @last`, map[string]interface{}{
		"uid":    uid,
		"before": before,
		"last":   last,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build notifications sql query: %v", err)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select notifications: %v", err)
	}

	defer rows.Close()

	nn := make([]Notification, 0, last)
	for rows.Next() {
		var n Notification
		if err = rows.Scan(&n.ID, pq.Array(&n.Actors), &n.Type, &n.PostID, &n.Read, &n.IssuedAt); err != nil {
			return nil, fmt.Errorf("could not scan notification: %v", err)
		}

		nn = append(nn, n)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not iterate over notification rows: %v", err)
	}

	return nn, nil
}

// SubscribeToNotifications to receive notifications in realtime.
func (s *Service) SubscribeToNotifications(ctx context.Context) (chan Notification, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(int64)
	if !ok {
		return nil, ErrUnauthenticated
	}

	topic := fmt.Sprintf("notification:%d", uid)
	nn := make(chan Notification)
	unsub, err := s.pubsub.Sub(topic, func(b []byte) {
		var pb pb.Notification
		if err := proto.Unmarshal(b, &pb); err != nil {
			log.Printf("could not unmarshal notification pb: %v\n", err)
			return
		}

		n := notificationFromPB(&pb)
		nn <- *n
	})

	if err != nil {
		return nil, fmt.Errorf("could not subscribe to notifications: %v", err)
	}

	go func() {
		<-ctx.Done()
		if err := unsub(); err != nil {
			log.Printf("could not unsubscribe from notifications: %v\n", err)
		}
		close(nn)
	}()

	return nn, nil
}

// HasUnreadNotifications checks if the authenticated user has any unread notification.
func (s *Service) HasUnreadNotifications(ctx context.Context) (bool, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(int64)
	if !ok {
		return false, ErrUnauthenticated
	}

	var unread bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM notifications WHERE user_id = $1 AND read = false
	)`, uid).Scan(&unread); err != nil {
		return false, fmt.Errorf("could not query select unread notifications existence: %v", err)
	}

	return unread, nil
}

// MarkNotificationAsRead sets a notification from the authenticated user as read.
func (s *Service) MarkNotificationAsRead(ctx context.Context, notificationID int64) error {
	uid, ok := ctx.Value(KeyAuthUserID).(int64)
	if !ok {
		return ErrUnauthenticated
	}

	if _, err := s.db.Exec(`
		UPDATE notifications SET read = true
		WHERE id = $1 AND user_id = $2`, notificationID, uid); err != nil {
		return fmt.Errorf("could not update and mark notification as read: %v", err)
	}

	return nil
}

// MarkNotificationsAsRead sets all notification from the authenticated user as read.
func (s *Service) MarkNotificationsAsRead(ctx context.Context) error {
	uid, ok := ctx.Value(KeyAuthUserID).(int64)
	if !ok {
		return ErrUnauthenticated
	}

	if _, err := s.db.Exec(`
		UPDATE notifications SET read = true
		WHERE user_id = $1`, uid); err != nil {
		return fmt.Errorf("could not update and mark notifications as read: %v", err)
	}

	return nil
}

func (s *Service) notifyFollow(followerID, followeeID int64) {
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("could not begin tx: %v\n", err)
		return
	}

	defer tx.Rollback()

	var actor string
	query := "SELECT username FROM users WHERE id = $1"
	if err = tx.QueryRow(query, followerID).Scan(&actor); err != nil {
		log.Printf("could not query select follow notification actor: %v\n", err)
		return
	}

	var notified bool
	query = `SELECT EXISTS (
		SELECT 1 FROM notifications
		WHERE user_id = $1
			AND $2:::VARCHAR = ANY(actors)
			AND type = 'follow'
	)`
	if err = tx.QueryRow(query, followeeID, actor).Scan(&notified); err != nil {
		log.Printf("could not query select follow notification existence: %v\n", err)
		return
	}

	if notified {
		return
	}

	var nid int64
	query = "SELECT id FROM notifications WHERE user_id = $1 AND type = 'follow' AND read = false"
	err = tx.QueryRow(query, followeeID).Scan(&nid)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("could not query select unread follow notification: %v\n", err)
		return
	}

	var n Notification
	if err == sql.ErrNoRows {
		actors := []string{actor}
		query = `
			INSERT INTO notifications (user_id, actors, type) VALUES ($1, $2, 'follow')
			RETURNING id, issued_at`
		if err = tx.QueryRow(query, followeeID, pq.Array(actors)).Scan(&n.ID, &n.IssuedAt); err != nil {
			log.Printf("could not insert follow notification: %v\n", err)
			return
		}

		n.Actors = actors
	} else {
		query = `
			UPDATE notifications SET
				actors = array_prepend($1, notifications.actors),
				issued_at = now()
			WHERE id = $2
			RETURNING actors, issued_at`
		if err = tx.QueryRow(query, actor, nid).Scan(pq.Array(&n.Actors), &n.IssuedAt); err != nil {
			log.Printf("could not update follow notification: %v\n", err)
			return
		}

		n.ID = nid
	}

	n.UserID = followeeID
	n.Type = "follow"

	if err = tx.Commit(); err != nil {
		log.Printf("could not commit to notify follow: %v\n", err)
		return
	}

	go s.broadcastNotification(n)
}

func (s *Service) notifyComment(c Comment) {
	actor := c.User.Username
	rows, err := s.db.Query(`
		INSERT INTO notifications (user_id, actors, type, post_id)
		SELECT user_id, $1, 'comment', $2 FROM post_subscriptions
		WHERE post_subscriptions.user_id != $3
			AND post_subscriptions.post_id = $2
		ON CONFLICT (user_id, type, post_id, read) DO UPDATE SET
			actors = array_prepend($4, array_remove(notifications.actors, $4)),
			issued_at = now()
		RETURNING id, user_id, actors, issued_at`,
		pq.Array([]string{actor}),
		c.PostID,
		c.UserID,
		actor,
	)
	if err != nil {
		log.Printf("could not insert comment notifications: %v\n", err)
		return
	}

	defer rows.Close()

	for rows.Next() {
		var n Notification
		if err = rows.Scan(&n.ID, &n.UserID, pq.Array(&n.Actors), &n.IssuedAt); err != nil {
			log.Printf("could not scan comment notification: %v\n", err)
			return
		}

		n.Type = "comment"
		n.PostID = &c.PostID

		go s.broadcastNotification(n)
	}

	if err = rows.Err(); err != nil {
		log.Printf("could not iterate over comment notification rows: %v\n", err)
		return
	}
}

func (s *Service) notifyPostMention(p Post) {
	mentions := collectMentions(p.Content)
	if len(mentions) == 0 {
		return
	}

	actors := []string{p.User.Username}
	rows, err := s.db.Query(`
		INSERT INTO notifications (user_id, actors, type, post_id)
		SELECT users.id, $1, 'post_mention', $2 FROM users
		WHERE users.id != $3
			AND username = ANY($4)
		RETURNING id, user_id, issued_at`,
		pq.Array(actors),
		p.ID,
		p.UserID,
		pq.Array(mentions),
	)
	if err != nil {
		log.Printf("could not insert post mention notifications: %v\n", err)
		return
	}

	defer rows.Close()

	for rows.Next() {
		var n Notification
		if err = rows.Scan(&n.ID, &n.UserID, &n.IssuedAt); err != nil {
			log.Printf("could not scan post mention notification: %v\n", err)
			return
		}

		n.Actors = actors
		n.Type = "post_mention"
		n.PostID = &p.ID

		go s.broadcastNotification(n)
	}

	if err = rows.Err(); err != nil {
		log.Printf("could not iterate post mention notification rows: %v\n", err)
		return
	}
}

func (s *Service) notifyCommentMention(c Comment) {
	mentions := collectMentions(c.Content)
	if len(mentions) == 0 {
		return
	}

	actor := c.User.Username
	rows, err := s.db.Query(`
		INSERT INTO notifications (user_id, actors, type, post_id)
		SELECT users.id, $1, 'comment_mention', $2 FROM users
		WHERE users.id != $3
			AND username = ANY($4)
		ON CONFLICT (user_id, type, post_id, read) DO UPDATE SET
			actors = array_prepend($5, array_remove(notifications.actors, $5)),
			issued_at = now()
		RETURNING id, user_id, actors, issued_at`,
		pq.Array([]string{actor}),
		c.PostID,
		c.UserID,
		pq.Array(mentions),
		actor,
	)
	if err != nil {
		log.Printf("could not insert comment mention notifications: %v\n", err)
		return
	}

	defer rows.Close()

	for rows.Next() {
		var n Notification
		if err = rows.Scan(&n.ID, &n.UserID, pq.Array(&n.Actors), &n.IssuedAt); err != nil {
			log.Printf("could not scan comment mention notification: %v\n", err)
			return
		}

		n.Type = "comment_mention"
		n.PostID = &c.PostID

		go s.broadcastNotification(n)
	}

	if err = rows.Err(); err != nil {
		log.Printf("could not iterate comment mention notification rows: %v\n", err)
		return
	}
}

func (s *Service) broadcastNotification(n Notification) {
	b, err := proto.Marshal(n.PB())
	if err != nil {
		log.Printf("could not marshal notification pb: %v\n", err)
		return
	}

	topic := fmt.Sprintf("notification:%d", n.UserID)
	if err = s.pubsub.Pub(topic, b); err != nil {
		log.Printf("could not broadcast notification: %v\n", err)
	}
}

// PB is the protocol buffer representation.
func (n *Notification) PB() *pb.Notification {
	if n == nil {
		return nil
	}

	pb := pb.Notification{
		Id:     n.ID,
		UserId: n.UserID,
		Actors: n.Actors,
		Type:   n.Type,
		Read:   n.Read,
	}
	if n.PostID != nil {
		pb.PostId = *n.PostID
	}
	issuedAt, err := ptypes.TimestampProto(n.IssuedAt)
	if err == nil {
		pb.IssuedAt = issuedAt
	}
	return &pb
}

func notificationFromPB(pb *pb.Notification) *Notification {
	if pb == nil {
		return nil
	}

	n := Notification{
		ID:     pb.GetId(),
		UserID: pb.GetUserId(),
		Actors: pb.GetActors(),
		Type:   pb.GetType(),
		Read:   pb.GetRead(),
	}
	postID := pb.GetPostId()
	if postID > 0 {
		n.PostID = &postID
	}
	issuedAt, err := ptypes.Timestamp(pb.GetIssuedAt())
	if err == nil {
		n.IssuedAt = issuedAt
	}
	return &n
}
