package types

import (
	"time"

	"github.com/nicolasparada/go-errs"
)

type Post struct {
	ID            string     `json:"id"`
	UserID        string     `json:"userID" db:"user_id"`
	Content       string     `json:"content"`
	SpoilerOf     *string    `json:"spoilerOf" db:"spoiler_of"`
	NSFW          bool       `json:"nsfw"`
	Reactions     []Reaction `json:"reactions"`
	CommentsCount int        `json:"commentsCount" db:"comments_count"`
	MediaURLs     []string   `json:"mediaURLs" db:"media"`
	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time  `json:"updatedAt" db:"updated_at"`
	User          *User      `json:"user,omitempty"`
	Mine          bool       `json:"mine" db:"mine,omitempty"`
	Subscribed    bool       `json:"subscribed" db:"subscribed,omitempty"`
}

func (p *Post) SetMediaURLs(prefix string) {
	for i, media := range p.MediaURLs {
		p.MediaURLs[i] = joinPrefix(prefix, media)
	}
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

type UpdatePost struct {
	Content   *string `json:"content"`
	SpoilerOf *string `json:"spoilerOf"`
	NSFW      *bool   `json:"nsfw"`
}

func (params UpdatePost) Empty() bool {
	return params.Content == nil && params.NSFW == nil && params.SpoilerOf == nil
}

type UpdatedPost struct {
	Content   string    `json:"content"`
	SpoilerOf *string   `json:"spoilerOf"`
	NSFW      bool      `json:"nsfw"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type PostsOpts struct {
	Username *string
	Tag      *string
}

type PostsOpt func(*PostsOpts)

func PostsFromUser(username string) PostsOpt {
	return func(opts *PostsOpts) {
		opts.Username = &username
	}
}

func PostsTagged(tag string) PostsOpt {
	return func(opts *PostsOpts) {
		opts.Tag = &tag
	}
}
