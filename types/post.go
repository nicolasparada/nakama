package types

import (
	"io"
	"time"
	"unicode/utf8"

	"github.com/nakamauwu/nakama/textutil"
	"github.com/nicolasparada/go-errs"
)

const (
	PostContentMaxLength = 2048
	PostSpoilerMaxLength = 64
)

type Post struct {
	ID            string     `json:"id"`
	UserID        string     `json:"userID" db:"user_id"`
	Content       string     `json:"content"`
	SpoilerOf     *string    `json:"spoilerOf" db:"spoiler_of"`
	NSFW          bool       `json:"nsfw"`
	MediaURLs     []string   `json:"mediaURLs" db:"media"`
	Reactions     []Reaction `json:"reactions"`
	CommentsCount int        `json:"commentsCount" db:"comments_count"`
	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time  `json:"updatedAt" db:"updated_at"`
	User          *User      `json:"user,omitempty"`
	Mine          bool       `json:"mine" db:"mine,omitempty"`
	Subscribed    bool       `json:"subscribed" db:"subscribed,omitempty"`
}

func (p *Post) SetMediaURLs(base, bucket string) {
	for i, media := range p.MediaURLs {
		p.MediaURLs[i] = makeURL(base, bucket, media)
	}
}

type PostPreview struct {
	ID        string   `json:"id"`
	UserID    string   `json:"userID"`
	Content   string   `json:"content"`
	SpoilerOf *string  `json:"spoilerOf"`
	NSFW      bool     `json:"nsfw"`
	MediaURLs []string `json:"mediaURLs"`

	Mine bool `json:"mine"`
}

func (p *PostPreview) SetMediaURLs(base, bucket string) {
	for i, media := range p.MediaURLs {
		p.MediaURLs[i] = makeURL(base, bucket, media)
	}
}

type CreatePost struct {
	Content      string          `json:"content"`
	SpoilerOf    *string         `json:"spoilerOf"`
	NSFW         bool            `json:"nsfw"`
	MediaReaders []io.ReadSeeker `json:"-"`
	userID       string
	tags         []string
	media        []string
}

func (in *CreatePost) SetUserID(userID string) {
	in.userID = userID
}

func (in CreatePost) UserID() string {
	return in.userID
}

func (in *CreatePost) SetTags(tags []string) {
	in.tags = tags
}

func (in CreatePost) Tags() []string {
	return in.tags
}

func (in *CreatePost) SetMedia(media []string) {
	in.media = media
}

func (in CreatePost) Media() []string {
	return in.media
}

func (in *CreatePost) Validate() error {
	in.Content = textutil.SmartTrim(in.Content)

	if in.Content == "" || utf8.RuneCountInString(in.Content) > PostContentMaxLength {
		return errs.InvalidArgumentError("invalid content")
	}

	if in.SpoilerOf != nil {
		*in.SpoilerOf = textutil.SmartTrim(*in.SpoilerOf)

		if *in.SpoilerOf == "" || utf8.RuneCountInString(*in.SpoilerOf) > PostSpoilerMaxLength {
			return errs.InvalidArgumentError("invalid spoiler")
		}
	}

	return nil
}

type ListPosts struct {
	Username *string
	Tag      *string
	PageArgs
	viewerID *string
}

func (in *ListPosts) SetViewerID(userID string) {
	in.viewerID = &userID
}

func (in ListPosts) ViewerID() *string {
	return in.viewerID
}

func (in *ListPosts) Validate() error {
	if in.Username != nil {
		if !ValidUsername(*in.Username) {
			return errs.InvalidArgumentError("invalid username")
		}
	}

	if in.Tag != nil {
		if *in.Tag == "" {
			return errs.InvalidArgumentError("invalid tag")
		}
	}

	return in.PageArgs.Validate()
}

type RetrievePost struct {
	PostID   string
	viewerID *string
}

func (in *RetrievePost) SetViewerID(userID string) {
	in.viewerID = &userID
}

func (in RetrievePost) ViewerID() *string {
	return in.viewerID
}

type UpdatePost struct {
	ID        string  `json:"-"`
	Content   *string `json:"content"`
	SpoilerOf *string `json:"spoilerOf"`
	NSFW      *bool   `json:"nsfw"`
	tags      []string
}

func (in *UpdatePost) SetTags(tags []string) {
	in.tags = tags
}

func (in UpdatePost) Tags() []string {
	return in.tags
}

func (in *UpdatePost) Validate() error {
	if !ValidUUIDv4(in.ID) {
		return errs.InvalidArgumentError("invalid post ID")
	}

	if in.Content != nil {
		*in.Content = textutil.SmartTrim(*in.Content)

		if *in.Content == "" || utf8.RuneCountInString(*in.Content) > PostContentMaxLength {
			return errs.InvalidArgumentError("invalid content")
		}
	}

	if in.SpoilerOf != nil {
		*in.SpoilerOf = textutil.SmartTrim(*in.SpoilerOf)

		if *in.SpoilerOf != "" && utf8.RuneCountInString(*in.SpoilerOf) > PostSpoilerMaxLength {
			return errs.InvalidArgumentError("invalid spoiler of")
		}
	}

	return nil
}

type UpdatedPost struct {
	Content   string    `json:"content"`
	SpoilerOf *string   `json:"spoilerOf" db:"spoiler_of"`
	NSFW      bool      `json:"nsfw"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}
