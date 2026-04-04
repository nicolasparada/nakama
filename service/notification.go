package service

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io"

	"github.com/nakamauwu/nakama/textutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

// Notifications from the authenticated user in descending order with backward pagination.
func (s *Service) Notifications(ctx context.Context, in types.ListNotifications) (types.Page[types.Notification], error) {
	var out types.Page[types.Notification]

	if err := in.Validate(); err != nil {
		return out, err
	}

	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, errs.Unauthenticated
	}

	in.SetUserID(uid)

	nn, err := s.Cockroach.Notifications(ctx, in)
	if err != nil {
		return out, err
	}

	for i, n := range nn.Items {
		for j, actor := range n.Actors {
			actor.SetAvatarURL(s.ObjectsBaseURL, AvatarsBucket)
			n.Actors[j] = actor
		}
		if n.Post != nil {
			n.Post.SetMediaPaths(s.ObjectsBaseURL, MediaBucket)
		}
		nn.Items[i] = n
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

	return s.Cockroach.HasUnreadNotifications(ctx, uid)
}

// MarkNotificationAsRead sets a notification from the authenticated user as read.
func (s *Service) MarkNotificationAsRead(ctx context.Context, notificationID string) error {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return errs.Unauthenticated
	}

	if !types.ValidUUIDv4(notificationID) {
		return errs.InvalidArgumentError("invalid notification ID")
	}

	return s.Cockroach.MarkNotificationAsRead(ctx, notificationID, uid)
}

// MarkNotificationsAsRead sets all notification from the authenticated user as read.
func (s *Service) MarkNotificationsAsRead(ctx context.Context) error {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return errs.Unauthenticated
	}

	return s.Cockroach.MarkNotificationsAsRead(ctx, uid)
}

func (s *Service) notifyFollow(followerID, followeeID string) {
	ctx := context.Background()
	notificationID, err := s.Cockroach.CreateFollowNotification(ctx, followeeID, followerID)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not create follow notification: %w", err))
		return
	}

	if notificationID == nil {
		return
	}

	n, err := s.notification(ctx, *notificationID)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not get follow notification: %w", err))
		return
	}

	go s.broadcastNotification(n)
}

func (s *Service) notifyComment(c types.Comment) {
	ctx := context.Background()
	createdList, err := s.Cockroach.FanoutCommentNotification(ctx, types.FanoutCommentNotification{
		ActorUserID: c.UserID,
		PostID:      c.PostID,
	})
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not fanout comment notification: %w", err))
		return
	}

	notifications, err := s.notificationsByIDs(ctx, collectNotificationIDs(createdList))
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not get notifications by IDs: %w", err))
		return
	}

	for _, n := range notifications {
		go s.broadcastNotification(n)
	}
}

func (s *Service) notifyPostMention(p types.Post) {
	ctx := context.Background()
	mentions := textutil.CollectMentions(p.Content)
	createdList, err := s.Cockroach.CreateMentionNotifications(ctx, types.CreateMentionNotifications{
		ActorUserID: p.UserID,
		PostID:      p.ID,
		Kind:        types.NotificationKindPostMention,
		Mentions:    mentions,
	})
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not create post mention notifications: %w", err))
		return
	}

	notifications, err := s.notificationsByIDs(ctx, collectNotificationIDs(createdList))
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not get notifications by IDs: %w", err))
		return
	}

	for _, n := range notifications {
		go s.broadcastNotification(n)
	}
}

func (s *Service) notifyCommentMention(c types.Comment) {
	ctx := context.Background()
	mentions := textutil.CollectMentions(c.Content)
	createdList, err := s.Cockroach.CreateMentionNotifications(ctx, types.CreateMentionNotifications{
		ActorUserID: c.UserID,
		PostID:      c.PostID,
		CommentID:   &c.ID,
		Kind:        types.NotificationKindCommentMention,
		Mentions:    mentions,
	})
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not create comment mention notifications: %w", err))
		return
	}

	notifications, err := s.notificationsByIDs(ctx, collectNotificationIDs(createdList))
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not get notifications by IDs: %w", err))
		return
	}

	for _, n := range notifications {
		go s.broadcastNotification(n)
	}
}

func (s *Service) notification(ctx context.Context, notificationID string) (types.Notification, error) {
	n, err := s.Cockroach.Notification(ctx, notificationID)
	if err != nil {
		return n, err
	}

	for i, actor := range n.Actors {
		actor.SetAvatarURL(s.ObjectsBaseURL, AvatarsBucket)
		n.Actors[i] = actor
	}
	if n.Post != nil {
		n.Post.SetMediaPaths(s.ObjectsBaseURL, MediaBucket)
	}

	return n, nil
}

func (s *Service) notificationsByIDs(ctx context.Context, ids []string) ([]types.Notification, error) {
	nn, err := s.Cockroach.NotificationsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	for i, n := range nn {
		for j, actor := range n.Actors {
			actor.SetAvatarURL(s.ObjectsBaseURL, AvatarsBucket)
			n.Actors[j] = actor
		}
		if n.Post != nil {
			n.Post.SetMediaPaths(s.ObjectsBaseURL, MediaBucket)
		}
		nn[i] = n
	}

	return nn, nil
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

func collectNotificationIDs(notifications []types.CreatedNotification) []string {
	ids := make([]string, len(notifications))
	for i, n := range notifications {
		ids[i] = n.ID
	}
	return ids
}

func notificationTopic(userID string) string { return "notification_" + userID }
