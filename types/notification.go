package types

import "time"

type Notification struct {
	ID     string           `db:"id"`
	UserID string           `db:"user_id"`
	Kind   NotificationKind `db:"kind"`
	// ActorUserIDs only includes the last 2 actors max.
	ActorUserIDs   []string        `db:"actor_user_ids"`
	ActorsCount    uint32          `db:"actors_count"`
	NotifiableKind *NotifiableKind `db:"notifiable_kind"`
	NotifiableID   *string         `db:"notifiable_id"`
	ReadAt         *time.Time      `db:"read_at"`
	CreatedAt      time.Time       `db:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"`

	// Actors only includes the last 2 actors max.
	Actors *[]User `db:"actors,omitempty"`

	Post    *Post    `db:"post,omitempty"`
	Comment *Comment `db:"comment,omitempty"`
}

func (n Notification) Read() bool {
	return n.ReadAt != nil
}

type NotificationKind string

func (k NotificationKind) String() string {
	return string(k)
}

const (
	NotificationKindFollow         NotificationKind = "follow"
	NotificationKindPostMention    NotificationKind = "post_mention"
	NotificationKindCommentMention NotificationKind = "comment_mention"
	NotificationKindComment        NotificationKind = "comment"
)

type NotifiableKind string

func (k NotifiableKind) String() string {
	return string(k)
}

const (
	NotifiableKindPost        NotifiableKind = "post"
	NotifiableKindComment     NotifiableKind = "comment"
	NotifiableKindPublication NotifiableKind = "publication"
	NotifiableKindChapter     NotifiableKind = "chapter"
)

type CreateFollowNotification struct {
	UserID      string
	ActorUserID string
}

type ListNotifications struct {
	PageArgs PageArgs

	userID string
}

func (in *ListNotifications) SetUserID(userID string) {
	in.userID = userID
}

func (in ListNotifications) UserID() string {
	return in.userID
}

type ReadNotification struct {
	NotificationID string
	userID         string
}

func (in *ReadNotification) SetUserID(userID string) {
	in.userID = userID
}

func (in ReadNotification) UserID() string {
	return in.userID
}

type CreateMentionNotifications struct {
	ActorUserID    string
	Kind           NotificationKind
	NotifiableKind NotifiableKind
	NotifiableID   string
	Usernames      []string
}

type FanoutCommentNotifications struct {
	ActorUserID string
	CommentID   string
}
