package types

import (
	"time"

	"github.com/nakamauwu/nakama/cursor"
)

type Post struct {
	ID            string     `json:"id"`
	UserID        string     `json:"-"`
	Content       string     `json:"content"`
	SpoilerOf     *string    `json:"spoilerOf" db:"spoiler_of"`
	NSFW          bool       `json:"nsfw"`
	Reactions     []Reaction `json:"reactions"`
	CommentsCount int        `json:"commentsCount" db:"comments_count"`
	MediaURLs     []string   `json:"mediaURLs" db:"media_urls"`
	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time  `json:"updatedAt" db:"updated_at"`
	User          *User      `json:"user,omitempty"`
	Mine          bool       `json:"mine" db:"mine,omitempty"`
	Subscribed    bool       `json:"subscribed" db:"subscribed,omitempty"`
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

type Posts []Post

func (pp Posts) EndCursor() *string {
	if len(pp) == 0 {
		return nil
	}

	last := pp[len(pp)-1]
	return new(cursor.Encode(last.ID, last.CreatedAt))
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
