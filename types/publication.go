package types

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/validator"
)

type Publication struct {
	ID          string          `db:"id"`
	UserID      string          `db:"user_id"`
	Kind        PublicationKind `db:"kind"`
	Title       string          `db:"title"`
	Description string          `db:"description"`
	CreatedAt   time.Time       `db:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at"`

	User *User `db:"user,omitempty"`
}

type PublicationKind string

const (
	PublicationKindManga    PublicationKind = "manga"
	PublicationKindNovel    PublicationKind = "novel"
	PublicationKindTutorial PublicationKind = "tutorial"
)

func PublicationKindFromPlural(plural string) PublicationKind {
	switch plural {
	case "manga":
		return PublicationKindManga
	case "novels":
		return PublicationKindNovel
	case "tutorials":
		return PublicationKindTutorial
	default:
		return ""
	}
}

func (k PublicationKind) Valid() bool {
	switch k {
	case PublicationKindManga, PublicationKindNovel, PublicationKindTutorial:
		return true
	}
	return false
}

func (k PublicationKind) String() string {
	return string(k)
}

func (k PublicationKind) Plural() string {
	switch k {
	case PublicationKindManga:
		return "manga"
	case PublicationKindNovel:
		return "novels"
	case PublicationKindTutorial:
		return "tutorials"
	default:
		return "publications"
	}
}

type CreatePublication struct {
	userID      string
	Kind        PublicationKind
	Title       string
	Description string
}

func (in *CreatePublication) SetUserID(userID string) {
	in.userID = userID
}

func (in CreatePublication) UserID() string {
	return in.userID
}

func (in *CreatePublication) Validate() error {
	v := validator.New()

	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)

	if !in.Kind.Valid() {
		v.AddError("Kind", "Invalid publication kind")
	}

	if in.Title == "" {
		v.AddError("Title", "Title cannot be empty")
	}
	if utf8.RuneCountInString(in.Title) > 100 {
		v.AddError("Title", "Title cannot exceed 100 characters")
	}

	if in.Description == "" {
		v.AddError("Description", "Description cannot be empty")
	}
	if utf8.RuneCountInString(in.Description) > 500 {
		v.AddError("Description", "Description cannot exceed 500 characters")
	}

	return v.AsError()
}

type ListPublications struct {
	Kind PublicationKind
}

func (in *ListPublications) Validate() error {
	v := validator.New()

	if !in.Kind.Valid() {
		v.AddError("Kind", "Invalid publication kind")
	}

	return v.AsError()
}
