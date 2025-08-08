package types

import (
	"time"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/validator"
)

type Comment struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	PostID    string    `db:"post_id"`
	Content   string    `db:"content"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	User *User `db:"user,omitempty"`
}

type CreateComment struct {
	userID  string
	PostID  string
	Content string
}

func (in *CreateComment) SetUserID(userID string) {
	in.userID = userID
}

func (in CreateComment) UserID() string {
	return in.userID
}

func (in *CreateComment) Validate() error {
	v := validator.New()

	if !id.Valid(in.PostID) {
		v.AddError("PostID", "PostID must be a valid ID")
	}

	if in.Content == "" {
		v.AddError("Content", "Content cannot be empty")
	}
	if utf8.RuneCountInString(in.Content) > 500 {
		v.AddError("Content", "Content cannot exceed 500 characters")
	}

	return v.AsError()
}
