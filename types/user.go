package types

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/nicolasparada/go-errs"
)

type User struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatarURL" db:"avatar"`
}

func (u *User) SetAvatarURL(baseURL, bucket string) {
	u.AvatarURL = optionalURL(baseURL, bucket, u.AvatarURL)
}

type UserProfile struct {
	User
	Email            string  `json:"email,omitempty"`
	CoverURL         *string `json:"coverURL" db:"cover"`
	Bio              *string `json:"bio"`
	Waifu            *string `json:"waifu"`
	Husbando         *string `json:"husbando"`
	FollowersCount   int     `json:"followersCount" db:"followers_count"`
	FolloweesCount   int     `json:"followeesCount" db:"followees_count"`
	IsMe             bool    `json:"isMe" db:"is_me"`
	FollowedByViewer bool    `json:"followedByViewer" db:"followed_by_viewer"`
	FollowsViewer    bool    `json:"followsViewer" db:"follows_viewer"`
}

func (u *UserProfile) SetCoverURL(base, bucket string) {
	u.CoverURL = optionalURL(base, bucket, u.CoverURL)
}

type CreateUser struct {
	Email    string
	Username string
	Provider *Provider
}

type ToggledFollow struct {
	FollowedByViewer bool `json:"followedByViewer"`
	FollowersCount   uint `json:"followersCount"`
}

type ListUserProfiles struct {
	SearchUsername *string
	PageArgs
	viewerID *string
}

func (in *ListUserProfiles) SetViewerID(viewerID string) {
	in.viewerID = &viewerID
}

func (in ListUserProfiles) ViewerID() *string {
	return in.viewerID
}

func (in *ListUserProfiles) Validate() error {
	if in.SearchUsername != nil {
		*in.SearchUsername = strings.TrimSpace(*in.SearchUsername)
		if *in.SearchUsername == "" {
			return errs.InvalidArgumentError("invalid search username")
		}
	}

	return in.PageArgs.Validate()
}

type ListFollowers struct {
	Username string
	PageArgs
	viewerID *string
}

func (in *ListFollowers) SetViewerID(viewerID string) {
	in.viewerID = &viewerID
}

func (in ListFollowers) ViewerID() *string {
	return in.viewerID
}

func (in *ListFollowers) Validate() error {
	in.Username = strings.TrimSpace(in.Username)
	if !ValidUsername(in.Username) {
		return errs.InvalidArgumentError("invalid username")
	}

	return in.PageArgs.Validate()
}

type ListFollowees struct {
	Username string
	PageArgs
	viewerID *string
}

func (in *ListFollowees) SetViewerID(viewerID string) {
	in.viewerID = &viewerID
}

func (in ListFollowees) ViewerID() *string {
	return in.viewerID

}

func (in *ListFollowees) Validate() error {
	in.Username = strings.TrimSpace(in.Username)
	if !ValidUsername(in.Username) {
		return errs.InvalidArgumentError("invalid username")
	}

	return in.PageArgs.Validate()
}

type ListUsernames struct {
	StartingWith string
	PageArgs
	viwerID *string
}

func (in *ListUsernames) SetViewerID(viewerID string) {
	in.viwerID = &viewerID
}

func (in ListUsernames) ViewerID() *string {
	return in.viwerID
}

func (in *ListUsernames) Validate() error {
	if strings.TrimSpace(in.StartingWith) == "" {
		return errs.InvalidArgumentError("invalid starting with")
	}

	return in.PageArgs.Validate()
}

type RetrieveUserProfile struct {
	Username string
	viewerID *string
}

func (in *RetrieveUserProfile) SetViewerID(viewerID string) {
	in.viewerID = &viewerID
}

func (in RetrieveUserProfile) ViewerID() *string {
	return in.viewerID
}

func (in *RetrieveUserProfile) Validate() error {
	in.Username = strings.TrimSpace(in.Username)
	if !ValidUsername(in.Username) {
		return errs.InvalidArgumentError("invalid username")
	}

	return nil
}

type UpdateUser struct {
	Username *string `json:"username"`
	Bio      *string `json:"bio"`
	Waifu    *string `json:"waifu"`
	Husbando *string `json:"husbando"`
	userID   string
}

func (u *UpdateUser) SetUserID(userID string) {
	u.userID = userID
}

func (u UpdateUser) UserID() string {
	return u.userID
}

func (u *UpdateUser) Validate() error {
	if u.Username != nil {
		*u.Username = strings.TrimSpace(*u.Username)
		if !ValidUsername(*u.Username) {
			return errs.InvalidArgumentError("invalid username")
		}
	}

	if u.Bio != nil {
		*u.Bio = strings.TrimSpace(*u.Bio)
		if !validUserBio(*u.Bio) {
			return errs.InvalidArgumentError("invalid user bio")
		}
	}

	if u.Waifu != nil {
		*u.Waifu = strings.TrimSpace(*u.Waifu)
		if !validAnimeCharName(*u.Waifu) {
			return errs.InvalidArgumentError("invalid waifu name")
		}
	}

	if u.Husbando != nil {
		*u.Husbando = strings.TrimSpace(*u.Husbando)
		if !validAnimeCharName(*u.Husbando) {
			return errs.InvalidArgumentError("invalid husbando name")
		}
	}

	return nil
}

func optionalURL(base, bucket string, key *string) *string {
	if key == nil || strings.HasPrefix(*key, base) {
		return nil
	}

	base = strings.TrimRight(base, "/")
	bucket = strings.Trim(bucket, "/")
	keyClean := strings.TrimLeft(*key, "/")

	return new(fmt.Sprintf("%s/%s/%s", base, bucket, keyClean))
}

func makeURL(base, bucket, key string) string {
	if strings.HasPrefix(key, base) {
		return key
	}

	base = strings.TrimRight(base, "/")
	bucket = strings.Trim(bucket, "/")
	key = strings.TrimLeft(key, "/")

	return fmt.Sprintf("%s/%s/%s", base, bucket, key)
}

var reEmail = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func ValidEmail(s string) bool {
	return reEmail.MatchString(s)
}

var reUsername = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,17}$`)

func ValidUsername(s string) bool {
	return reUsername.MatchString(s)
}

func validUserBio(s string) bool {
	return s != "" && utf8.RuneCountInString(s) <= 480
}

func validAnimeCharName(s string) bool {
	return s != "" && utf8.RuneCountInString(s) <= 32
}
