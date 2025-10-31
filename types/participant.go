package types

import "time"

type Participant struct {
	UserID         string            `db:"user_id"`
	ChatID         string            `db:"chat_id"`
	OtherUserID    string            `db:"other_user_id"`
	Status         ParticipantStatus `db:"status"`
	HasUnread      bool              `db:"has_unread"`
	LastReadAt     *time.Time        `db:"last_read_at"`
	LastActivityAt *time.Time        `db:"last_activity_at"`
	CreatedAt      time.Time         `db:"created_at"`
	UpdatedAt      time.Time         `db:"updated_at"`

	OtherUser *User `db:"other_user,omitempty"`
}

type ParticipantStatus string

const (
	ParticipantStatusActive          ParticipantStatus = "active"
	ParticipantStatusPendingSender   ParticipantStatus = "pending_sender"   // user who initiated the conversation
	ParticipantStatusPendingReceiver ParticipantStatus = "pending_receiver" // user who received the conversation request
)

func (ps ParticipantStatus) String() string {
	return string(ps)
}
