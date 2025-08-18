package types

import (
	"time"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/emoji"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/validator"
)

type Reaction struct {
	UserID    string    `db:"user_id"`
	PostID    string    `db:"post_id"`
	Emoji     string    `db:"emoji"`
	CreatedAt time.Time `db:"created_at"`
}

type ReactionCounters []ReactionCounter

type ReactionCounter struct {
	Emoji   string `json:"emoji"`
	Count   uint64 `json:"count"`
	Reacted bool   `json:"reacted"`
}

type ToggleReaction struct {
	PostID string
	Emoji  string

	loggedInUserID string
}

func (in *ToggleReaction) SetLoggedInUserID(userID string) {
	in.loggedInUserID = userID
}

func (in ToggleReaction) LoggedInUserID() string {
	return in.loggedInUserID
}

func (in *ToggleReaction) Validate() error {
	v := validator.New()

	if !id.Valid(in.PostID) {
		v.AddError("PostID", "PostID must be a valid ID")
	}

	if in.Emoji == "" {
		v.AddError("Emoji", "Emoji cannot be empty")
	}
	if utf8.RuneCountInString(in.Emoji) > 32 {
		v.AddError("Emoji", "Emoji must be at most 32 characters")
	}
	if !emoji.IsValid(in.Emoji) {
		v.AddError("Emoji", "Invalid emoji format")
	}

	return v.AsError()
}

type ToggleCommentReaction struct {
	CommentID string
	Emoji     string

	loggedInUserID string
}

func (in *ToggleCommentReaction) SetLoggedInUserID(userID string) {
	in.loggedInUserID = userID
}

func (in ToggleCommentReaction) LoggedInUserID() string {
	return in.loggedInUserID
}

func (in *ToggleCommentReaction) Validate() error {
	v := validator.New()

	if !id.Valid(in.CommentID) {
		v.AddError("CommentID", "CommentID must be a valid ID")
	}

	if in.Emoji == "" {
		v.AddError("Emoji", "Emoji cannot be empty")
	}
	if utf8.RuneCountInString(in.Emoji) > 32 {
		v.AddError("Emoji", "Emoji must be at most 32 characters")
	}
	if !emoji.IsValid(in.Emoji) {
		v.AddError("Emoji", "Invalid emoji format")
	}

	return v.AsError()
}
