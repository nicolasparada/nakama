package types

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/validator"
)

type Chat struct {
	ID        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`

	Participation *Participant `db:"participation,omitempty"`
}

type RetrieveChat struct {
	ChatID string

	loggedInUserID string
}

func (in *RetrieveChat) SetLoggedInUserID(userID string) {
	in.loggedInUserID = userID
}

func (in RetrieveChat) LoggedInUserID() string {
	return in.loggedInUserID
}

func (in *RetrieveChat) Validate() error {
	v := validator.New()

	if in.ChatID == "" {
		v.AddError("ChatID", "Chat ID is required")
	}
	if !id.Valid(in.ChatID) {
		v.AddError("ChatID", "Chat ID is invalid")
	}

	return v.AsError()
}

type RetrieveChatFromParticipants struct {
	OtherUserID string

	loggedInUserID string
}

func (in *RetrieveChatFromParticipants) SetLoggedInUserID(userID string) {
	in.loggedInUserID = userID
}

func (in RetrieveChatFromParticipants) LoggedInUserID() string {
	return in.loggedInUserID
}

func (in *RetrieveChatFromParticipants) Validate() error {
	v := validator.New()

	if in.OtherUserID == "" {
		v.AddError("OtherUserID", "Other user ID is required")
	}
	if !id.Valid(in.OtherUserID) {
		v.AddError("OtherUserID", "Other user ID is invalid")
	}

	return v.AsError()
}

type CreateChat struct {
	OtherUserID string
	Content     string

	loggedInUserID     string
	followingEachOther bool
	chatID             string
}

func (in *CreateChat) SetLoggedInUserID(userID string) {
	in.loggedInUserID = userID
}

func (in CreateChat) LoggedInUserID() string {
	return in.loggedInUserID
}

func (in *CreateChat) SetFollowingEachOther(followingEachOther bool) {
	in.followingEachOther = followingEachOther
}

func (in CreateChat) FollowingEachOther() bool {
	return in.followingEachOther
}

func (in *CreateChat) SetChatID(chatID string) {
	in.chatID = chatID
}

func (in CreateChat) ChatID() string {
	return in.chatID
}

func (in *CreateChat) Validate() error {
	v := validator.New()

	in.Content = strings.TrimSpace(in.Content)

	if in.OtherUserID == "" {
		v.AddError("OtherUserID", "Other user ID is required")
	}
	if !id.Valid(in.OtherUserID) {
		v.AddError("OtherUserID", "Other user ID is invalid")
	}

	if in.Content == "" {
		v.AddError("Content", "Content is required")
	}
	if utf8.RuneCountInString(in.Content) > 1000 {
		v.AddError("Content", "Content must be less than 1000 characters")
	}

	return v.AsError()
}

type ListChats struct {
	PageArgs PageArgs

	loggedInUserID string
}

func (in *ListChats) SetLoggedInUserID(userID string) {
	in.loggedInUserID = userID
}

func (in ListChats) LoggedInUserID() string {
	return in.loggedInUserID
}
