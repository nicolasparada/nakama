package types

import (
	"time"
	"unicode/utf8"

	"github.com/nakamauwu/nakama/cursor"
	"github.com/nakamauwu/nakama/textutil"
	"github.com/nicolasparada/go-errs"
)

const CommentContentMaxLength = 2048

type Comment struct {
	ID        string     `json:"id"`
	UserID    string     `json:"userID" db:"user_id"`
	PostID    string     `json:"postID" db:"post_id"`
	Content   string     `json:"content"`
	Reactions []Reaction `json:"reactions"`
	CreatedAt time.Time  `json:"createdAt" db:"created_at"`
	User      *User      `json:"user,omitempty"`
	Mine      bool       `json:"mine" db:"mine,omitempty"`
}

type CreateComment struct {
	PostID  string `json:"-"`
	Content string `json:"content"`

	userID string
	tags   []string
}

func (in *CreateComment) SetUserID(userID string) {
	in.userID = userID
}

func (in CreateComment) UserID() string {
	return in.userID
}

func (in *CreateComment) SetTags(tags []string) {
	in.tags = tags
}

func (in CreateComment) Tags() []string {
	return in.tags
}

func (in *CreateComment) Validate() error {
	if !ValidUUIDv4(in.PostID) {
		return errs.InvalidArgumentError("invalid post ID")
	}

	in.Content = textutil.SmartTrim(in.Content)
	if in.Content == "" || utf8.RuneCountInString(in.Content) > CommentContentMaxLength {
		return errs.InvalidArgumentError("invalid content")
	}

	return nil
}

type ListComments struct {
	PostID string
	PageArgs
	viewerID *string
}

func (in *ListComments) SetViewerID(userID string) {
	in.viewerID = &userID
}

func (in ListComments) ViewerID() *string {
	return in.viewerID
}

func (in *ListComments) Validate() error {
	if !ValidUUIDv4(in.PostID) {
		return errs.InvalidArgumentError("invalid post ID")
	}

	return in.PageArgs.Validate()
}

type UpdateComment struct {
	ID      string  `json:"-"`
	Content *string `json:"content"`
}

type UpdatedComment struct {
	Content string `json:"content"`
}

type Comments []Comment

func (cc Comments) EndCursor() *string {
	if len(cc) == 0 {
		return nil
	}

	last := cc[len(cc)-1]
	return new(cursor.Encode(last.ID, last.CreatedAt))
}
