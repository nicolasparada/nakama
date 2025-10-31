package types

import (
	"time"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/validator"
)

type Message struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	ChatID    string    `db:"chat_id"`
	Content   string    `db:"content"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	User         *User                `db:"user,omitempty"`
	Relationship *MessageRelationship `db:"relationship,omitempty"`
}

type MessageRelationship struct {
	IsMine bool
}

type CreateMessage struct {
	ChatID  string
	Content string

	loggedInUserID string
}

func (in *CreateMessage) SetLoggedInUserID(userID string) {
	in.loggedInUserID = userID
}

func (in CreateMessage) LoggedInUserID() string {
	return in.loggedInUserID
}

func (in *CreateMessage) Validate() error {
	v := validator.New()

	if in.ChatID == "" {
		v.AddError("ChatID", "ChatID is required")
	}
	if !id.Valid(in.ChatID) {
		v.AddError("ChatID", "ChatID is invalid")
	}
	if in.Content == "" {
		v.AddError("Content", "Content is required")
	}
	if utf8.RuneCountInString(in.Content) > 1000 {
		v.AddError("Content", "Content must be at most 1000 characters")
	}
	return v.AsError()
}

type ListMessages struct {
	ChatID   string
	PageArgs PageArgs

	loggedInUserID string
}

func (in *ListMessages) SetLoggedInUserID(userID string) {
	in.loggedInUserID = userID
}

func (in ListMessages) LoggedInUserID() string {
	return in.loggedInUserID
}

func (in *ListMessages) Validate() error {
	v := validator.New()

	if in.ChatID == "" {
		v.AddError("ChatID", "ChatID is required")
	}
	if !id.Valid(in.ChatID) {
		v.AddError("ChatID", "ChatID is invalid")
	}
	return v.AsError()
}
