package types

import (
	"time"
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
	ID           string           `json:"id"`
	UserID       string           `json:"userID" db:"user_id"`
	ActorUserIDs []string         `json:"actorUserIDs" db:"actor_user_ids"`
	ActorsCount  int              `json:"actorsCount" db:"actors_count"`
	Kind         NotificationKind `json:"kind" db:"kind"`
	PostID       *string          `json:"postID,omitempty" db:"post_id,omitempty"`
	CommentID    *string          `json:"commentID,omitempty" db:"comment_id,omitempty"`
	ReadAt       *time.Time       `json:"readAt" db:"read_at"`
	IssuedAt     time.Time        `json:"issuedAt" db:"issued_at"`
	Read         bool             `json:"read"`

	Actors  []User          `json:"actors"`
	Post    *PostPreview    `json:"post,omitempty"`
	Comment *CommentPreview `json:"comment,omitempty"`
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

type CreateNotification struct {
	UserID    string
	Kind      NotificationKind
	PostID    *string
	CommentID *string
}

type CreatedNotification struct {
	ID       string    `db:"id"`
	IssuedAt time.Time `db:"issued_at"`
}

type FanoutCommentNotification struct {
	ActorUserID string
	PostID      string
}

type CreateMentionNotifications struct {
	ActorUserID string
	PostID      string
	CommentID   *string
	Kind        NotificationKind
	Mentions    []string
}
