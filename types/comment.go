package types

import (
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/validator"
)

type Comment struct {
	ID               string           `db:"id"`
	UserID           string           `db:"user_id"`
	PostID           string           `db:"post_id"`
	Content          string           `db:"content"`
	Attachment       *Attachment      `db:"attachment"`
	ReactionCounters ReactionCounters `db:"reaction_counters"`
	CreatedAt        time.Time        `db:"created_at"`
	UpdatedAt        time.Time        `db:"updated_at"`

	User *User `db:"user,omitempty"`
}

type CreateComment struct {
	userID  string
	PostID  string
	Content string
	File    io.ReadSeeker

	attachment *Attachment
}

func (in *CreateComment) SetUserID(userID string) {
	in.userID = userID
}

func (in CreateComment) UserID() string {
	return in.userID
}

func (in *CreateComment) SetAttachment(attachment Attachment) {
	in.attachment = &attachment
}

func (in CreateComment) Attachment() *Attachment {
	return in.attachment
}

func (in *CreateComment) Validate() error {
	v := validator.New()

	in.Content = strings.TrimSpace(in.Content)

	if !id.Valid(in.PostID) {
		v.AddError("PostID", "PostID must be a valid ID")
	}

	if in.File == nil && in.Content == "" {
		v.AddError("Content", "Content cannot be empty")
	}
	if utf8.RuneCountInString(in.Content) > 500 {
		v.AddError("Content", "Content cannot exceed 500 characters")
	}

	return v.AsError()
}

type ListComments struct {
	PostID string

	loggedInUserID *string
}

func (in *ListComments) SetLoggedInUserID(userID string) {
	in.loggedInUserID = &userID
}

func (in ListComments) LoggedInUserID() *string {
	return in.loggedInUserID
}

func (in *ListComments) Validate() error {
	v := validator.New()

	if !id.Valid(in.PostID) {
		v.AddError("PostID", "PostID must be a valid ID")
	}

	return v.AsError()
}
