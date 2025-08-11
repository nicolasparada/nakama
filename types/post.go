package types

import (
	"io"
	"time"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/validator"
)

type Post struct {
	ID          string       `db:"id"`
	UserID      string       `db:"user_id"`
	Content     string       `db:"content"`
	IsR18       bool         `db:"is_r18"`
	Attachments []Attachment `db:"attachments"`
	CreatedAt   time.Time    `db:"created_at"`
	UpdatedAt   time.Time    `db:"updated_at"`

	User *User `db:"user,omitempty"`
}

type CreatePost struct {
	userID      string
	Content     string
	IsR18       bool
	Attachments []io.ReadSeeker

	processedAttachments []Attachment
}

func (in *CreatePost) SetUserID(userID string) {
	in.userID = userID
}

func (in CreatePost) UserID() string {
	return in.userID
}

func (in *CreatePost) SetProcessedAttachments(attachments []Attachment) {
	in.processedAttachments = attachments
}

func (in CreatePost) ProcessedAttachments() []Attachment {
	return in.processedAttachments
}

func (in *CreatePost) Validate() error {
	v := validator.New()

	if in.Content == "" {
		v.AddError("Content", "Content cannot be empty")
	}
	if utf8.RuneCountInString(in.Content) > 500 {
		v.AddError("Content", "Content cannot exceed 500 characters")
	}

	return v.AsError()
}
