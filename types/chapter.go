package types

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/validator"
)

type Chapter struct {
	ID            string    `db:"id"`
	PublicationID string    `db:"publication_id"`
	Title         string    `db:"title"`
	Content       string    `db:"content"`
	Number        uint32    `db:"number"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`

	Publication *Publication `db:"publication,omitempty"`
}

type CreateChapter struct {
	PublicationID string
	Title         string
	Content       string
	Number        uint32
}

func (in *CreateChapter) Validate() error {
	v := validator.New()

	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)

	if in.PublicationID == "" {
		v.AddError("PublicationID", "Publication ID cannot be empty")
	}
	if !id.Valid(in.PublicationID) {
		v.AddError("PublicationID", "Invalid publication ID")
	}

	if in.Title == "" {
		v.AddError("Title", "Title cannot be empty")
	}
	if utf8.RuneCountInString(in.Title) > 100 {
		v.AddError("Title", "Title cannot exceed 100 characters")
	}

	if in.Content == "" {
		v.AddError("Content", "Content cannot be empty")
	}

	if utf8.RuneCountInString(in.Content) > 10000 {
		v.AddError("Content", "Content cannot exceed 10,000 characters")
	}

	if in.Number == 0 {
		v.AddError("Number", "Chapter number must be greater than zero")
	}

	return v.AsError()
}
