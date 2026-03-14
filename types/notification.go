package types

import (
	"time"

	"github.com/nakamauwu/nakama/cursor"
)

type NotificationKind string

const (
	NotificationKindFollow         NotificationKind = "follow"
	NotificationKindComment        NotificationKind = "comment"
	NotificationKindPostMention    NotificationKind = "post_mention"
	NotificationKindCommentMention NotificationKind = "comment_mention"
)

func (k NotificationKind) IsValid() bool {
	switch k {
	case NotificationKindFollow, NotificationKindComment, NotificationKindPostMention, NotificationKindCommentMention:
		return true
	default:
		return false
	}
}

func (k NotificationKind) String() string {
	return string(k)
}

type Notification struct {
	ID             string           `json:"id"`
	UserID         string           `json:"-"`
	ActorUsernames []string         `json:"actorUsernames" db:"actor_usernames"`
	Kind           NotificationKind `json:"kind" db:"kind"`
	PostID         *string          `json:"postID,omitempty" db:"post_id,omitempty"`
	CommentID      *string          `json:"commentID,omitempty" db:"comment_id,omitempty"`
	ReadAt         *time.Time       `json:"readAt" db:"read_at"`
	IssuedAt       time.Time        `json:"issuedAt" db:"issued_at"`
	Read           bool             `json:"read"`

	Post    *PostPreview    `json:"post,omitempty"`
	Comment *CommentPreview `json:"comment,omitempty"`
}

type Notifications []Notification

func (pp Notifications) EndCursor() *string {
	if len(pp) == 0 {
		return nil
	}

	last := pp[len(pp)-1]
	return new(cursor.Encode(last.ID, last.IssuedAt))
}

type ListNotifications struct {
	PageArgs
	userID string
}

func (in *ListNotifications) SetUserID(userID string) {
	in.userID = userID
}

func (in ListNotifications) UserID() string {
	return in.userID
}

func (in *ListNotifications) Validate() error {
	return in.PageArgs.Validate()
}

type NotificationExists struct {
	UserID        *string
	ActorUsername *string
	Kind          *NotificationKind
	PostID        *string
}

func (in NotificationExists) IsEmpty() bool {
	var empty NotificationExists
	return in == empty
}

type CreateNotification struct {
	UserID         string
	ActorUsernames []string
	Kind           NotificationKind
	PostID         *string
}

type CreatedNotification struct {
	ID       string    `db:"id"`
	IssuedAt time.Time `db:"issued_at"`
}

type AddedNotificationActor struct {
	ActorUsernames []string  `db:"actor_usernames"`
	IssuedAt       time.Time `db:"issued_at"`
}

type FanoutCommentNotification struct {
	ActorUserID   string
	ActorUsername string
	PostID        string
	CommentID     string
}

type CreateMentionNotifications struct {
	ActorUserID   string
	ActorUsername string
	PostID        string
	CommentID     *string
	Kind          NotificationKind
	Mentions      []string
}
