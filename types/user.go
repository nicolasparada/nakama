package types

import (
	"regexp"
	"strings"

	"github.com/nakamauwu/nakama/cursor"
)

type User struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatarURL" db:"avatar"`
}

func (u *User) SetAvatarURL(prefix string) {
	u.AvatarURL = joinOptionalPrefix(prefix, u.AvatarURL)
}

type UserProfile struct {
	User
	Email          string  `json:"email,omitempty"`
	CoverURL       *string `json:"coverURL" db:"cover"`
	Bio            *string `json:"bio"`
	Waifu          *string `json:"waifu"`
	Husbando       *string `json:"husbando"`
	FollowersCount int     `json:"followersCount" db:"followers_count"`
	FolloweesCount int     `json:"followeesCount" db:"followees_count"`
	Me             bool    `json:"me"`
	Following      bool    `json:"following"`
	Followeed      bool    `json:"followeed"`
}

func (u *UserProfile) SetCoverURL(prefix string) {
	u.CoverURL = joinOptionalPrefix(prefix, u.CoverURL)
}

type ToggleFollowOutput struct {
	Following      bool `json:"following"`
	FollowersCount int  `json:"followersCount"`
}

type UserProfiles []UserProfile

func (uu UserProfiles) EndCursor() *string {
	if len(uu) == 0 {
		return nil
	}

	last := uu[len(uu)-1]
	return new(cursor.EncodeSimple(last.Username))
}

type Usernames []string

func (uu Usernames) EndCursor() *string {
	if len(uu) == 0 {
		return nil
	}

	last := uu[len(uu)-1]
	return new(cursor.EncodeSimple(last))
}

type UpdateUserParams struct {
	Username *string `json:"username"`
	Bio      *string `json:"bio"`
	Waifu    *string `json:"waifu"`
	Husbando *string `json:"husbando"`
}

func joinOptionalPrefix(prefix string, path *string) *string {
	if path == nil {
		return nil
	}

	if strings.HasPrefix(*path, prefix) {
		return path
	}

	url := prefix + *path
	return &url
}

func joinPrefix(prefix string, path string) string {
	if strings.HasPrefix(path, prefix) {
		return path
	}

	return prefix + path
}

var reEmail = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func ValidEmail(s string) bool {
	return reEmail.MatchString(s)
}

var reUsername = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,17}$`)

func ValidUsername(s string) bool {
	return reUsername.MatchString(s)
}
