package types

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/validator"
)

var (
	reEmail    = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	reUsername = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_\.-]*$`)
)

type User struct {
	ID             string      `db:"id"`
	Email          string      `db:"email"`
	Username       string      `db:"username"`
	Avatar         *Attachment `db:"avatar"`
	FollowersCount uint64      `db:"followers_count"`
	FollowingCount uint64      `db:"following_count"`
	CreatedAt      time.Time   `db:"created_at"`
	UpdatedAt      time.Time   `db:"updated_at"`

	Relationship *UserRelationship `db:"relationship,omitempty"`
}

type UserRelationship struct {
	FollowsYou    bool // Does this user follow the authenticated user? (for "follows you" badge)
	FollowedByYou bool // Does the authenticated user follow this user? (explicit state)
	IsMe          bool // Is this user the authenticated user? (for edit buttons, hiding follow buttons)
}

type UpsertUser struct {
	Email    string
	Username string
}

func (in *UpsertUser) Validate() error {
	v := validator.New()

	in.Email = strings.TrimSpace(in.Email)
	in.Email = strings.ToLower(in.Email)

	in.Username = strings.TrimSpace(in.Username)

	if in.Email == "" {
		v.AddError("Email", "Email is required")
	}
	if !reEmail.MatchString(in.Email) {
		v.AddError("Email", "Email is not valid")
	}
	if utf8.RuneCountInString(in.Email) > 254 {
		v.AddError("Email", "Email must be less than 255 characters")
	}

	if in.Username == "" {
		v.AddError("Username", "Username is required")
	}
	if !reUsername.MatchString(in.Username) {
		v.AddError("Username", "Username can only contain letters, numbers, and underscores")
	}
	if utf8.RuneCountInString(in.Username) > 21 {
		v.AddError("Username", "Username must be at most 21 characters")
	}

	return v.AsError()
}

type RetrieveUser struct {
	UserID         string
	loggedInUserID *string
}

func (in *RetrieveUser) SetLoggedInUserID(userID string) {
	in.loggedInUserID = &userID
}

func (in RetrieveUser) LoggedInUserID() *string {
	return in.loggedInUserID
}

func (in *RetrieveUser) Validate() error {
	v := validator.New()

	if in.UserID == "" {
		v.AddError("UserID", "User ID is required")
	}

	if !id.Valid(in.UserID) {
		v.AddError("UserID", "Invalid user ID format")
	}

	return v.AsError()
}

type RetrieveUserFromUsername struct {
	Username       string
	loggedInUserID *string
}

func (in *RetrieveUserFromUsername) SetLoggedInUserID(userID string) {
	in.loggedInUserID = &userID
}

func (in RetrieveUserFromUsername) LoggedInUserID() *string {
	return in.loggedInUserID
}

func (in *RetrieveUserFromUsername) Validate() error {
	v := validator.New()

	in.Username = strings.TrimSpace(in.Username)

	if in.Username == "" {
		v.AddError("Username", "Username is required")
	}
	if !reUsername.MatchString(in.Username) {
		v.AddError("Username", "Username can only contain letters, numbers, and underscores")
	}
	if utf8.RuneCountInString(in.Username) > 21 {
		v.AddError("Username", "Username must be less than 22 characters")
	}

	return v.AsError()
}

type ToggleFollow struct {
	FolloweeID     string
	loggedInUserID string
}

func (in *ToggleFollow) SetLoggedInUserID(userID string) {
	in.loggedInUserID = userID
}

func (in ToggleFollow) LoggedInUserID() string {
	return in.loggedInUserID
}

func (in *ToggleFollow) Validate() error {
	v := validator.New()

	if in.FolloweeID == "" {
		v.AddError("FolloweeID", "Followee ID is required")
	}

	if !id.Valid(in.FolloweeID) {
		v.AddError("FolloweeID", "Invalid followee ID format")
	}

	return v.AsError()
}

type UpdateUserAvatar struct {
	UserID string
	Avatar Attachment
}

type SearchUsers struct {
	Query    string
	PageArgs SimplePageArgs

	loggedInUserID *string
}

func (in *SearchUsers) SetLoggedInUserID(userID string) {
	in.loggedInUserID = &userID
}

func (in SearchUsers) LoggedInUserID() *string {
	return in.loggedInUserID
}
