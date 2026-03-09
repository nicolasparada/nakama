package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/jackc/pgx/v5"
	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/nakamauwu/nakama/cockroach"
	"github.com/nakamauwu/nakama/cursor"
	"github.com/nakamauwu/nakama/storage"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

const (
	// MaxAvatarBytes to read.
	MaxAvatarBytes = 5 << 20 // 5MB
	// MaxCoverBytes to read.
	MaxCoverBytes = 20 << 20 // 20MB

	AvatarsBucket = "avatars"
	CoversBucket  = "covers"
)

const (
	// ErrInvalidUserID denotes an invalid user id; that is not uuid.
	ErrInvalidUserID = errs.InvalidArgumentError("invalid user ID")
	// ErrInvalidEmail denotes an invalid email address.
	ErrInvalidEmail = errs.InvalidArgumentError("invalid email")
	// ErrInvalidUsername denotes an invalid username.
	ErrInvalidUsername = errs.InvalidArgumentError("invalid username")
	// ErrEmailTaken denotes an email already taken.
	ErrEmailTaken = errs.ConflictError("email taken")
	// ErrUsernameTaken denotes a username already taken.
	ErrUsernameTaken = errs.ConflictError("username taken")
	// ErrUserNotFound denotes a not found user.
	ErrUserNotFound = errs.NotFoundError("user not found")
	// ErrForbiddenFollow denotes a forbiden follow. Like following yourself.
	ErrForbiddenFollow = errs.PermissionDeniedError("forbidden follow")
	// ErrUnsupportedAvatarFormat denotes an unsupported avatar image format.
	ErrUnsupportedAvatarFormat = errs.InvalidArgumentError("unsupported avatar format")
	// ErrUnsupportedCoverFormat denotes an unsupported avatar image format.
	ErrUnsupportedCoverFormat = errs.InvalidArgumentError("unsupported cover format")
	// ErrUserGone denotes that the user has already been deleted.
	ErrUserGone = errs.GoneError("user gone")
	// ErrInvalidUpdateUserParams denotes invalid params to update a user, that is no params altogether.
	ErrInvalidUpdateUserParams = errs.InvalidArgumentError("invalid update user params")
	// ErrInvalidUserBio denotes an invalid user bio. That is empty or it exceeds the max allowed characters (480).
	ErrInvalidUserBio = errs.InvalidArgumentError("invalid user bio")
	// ErrInvalidUserWaifu denotes an invalid waifu name for an user.
	ErrInvalidUserWaifu = errs.InvalidArgumentError("invalid user waifu")
	// ErrInvalidUserHusbando denotes an invalid husbando name for an user.
	ErrInvalidUserHusbando = errs.InvalidArgumentError("invalid user husbando")
)

func (s *Service) UserProfiles(ctx context.Context, in types.ListUserProfiles) (types.Page[types.UserProfile], error) {
	var users types.Page[types.UserProfile]

	if err := in.Validate(); err != nil {
		return users, err
	}

	if uid, ok := ctx.Value(KeyAuthUserID).(string); ok {
		in.SetViewerID(uid)
	}

	users, err := s.Cockroach.UserProfiles(ctx, in)
	if err != nil {
		return users, err
	}

	for i, u := range users.Items {
		u.SetAvatarURL(s.AvatarURLPrefix)
		u.SetCoverURL(s.CoverURLPrefix)
		users.Items[i] = u
	}

	return users, nil

}

// Usernames to autocomplete a mention box or something.
func (s *Service) Usernames(ctx context.Context, in types.ListUsernames) (types.Page[string], error) {
	var out types.Page[string]

	if err := in.Validate(); err != nil {
		return out, err
	}

	if uid, ok := ctx.Value(KeyAuthUserID).(string); ok {
		in.SetViewerID(uid)
	}

	return s.Cockroach.Usernames(ctx, in)
}

func (s *Service) userByID(ctx context.Context, id string) (types.User, error) {
	user, err := s.Cockroach.User(ctx, id)
	if err != nil {
		return user, err
	}

	user.SetAvatarURL(s.AvatarURLPrefix)
	return user, nil
}

func (s *Service) userByEmail(ctx context.Context, email string) (types.User, error) {
	user, err := s.Cockroach.UserByEmail(ctx, email)
	if err != nil {
		return user, err
	}

	user.SetAvatarURL(s.AvatarURLPrefix)
	return user, nil
}

// UserProfileByUsername with the given username.
func (s *Service) UserProfileByUsername(ctx context.Context, in types.RetrieveUserProfile) (types.UserProfile, error) {
	var user types.UserProfile

	if err := in.Validate(); err != nil {
		return user, err
	}

	if uid, ok := ctx.Value(KeyAuthUserID).(string); ok {
		in.SetViewerID(uid)
	}

	user, err := s.Cockroach.UserProfileByUsername(ctx, in)
	if err != nil {
		return user, err
	}

	user.SetAvatarURL(s.AvatarURLPrefix)
	user.SetCoverURL(s.CoverURLPrefix)

	return user, nil
}

func (s *Service) UpdateUser(ctx context.Context, in types.UpdateUser) error {
	if err := in.Validate(); err != nil {
		return err
	}

	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return errs.Unauthenticated
	}

	in.SetUserID(uid)

	return s.Cockroach.UpdateUser(ctx, in)
}

// UpdateAvatar of the authenticated user returning the new avatar URL.
// Please limit the reader before hand using MaxAvatarBytes.
func (s *Service) UpdateAvatar(ctx context.Context, r io.ReadSeeker) (string, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return "", errs.Unauthenticated
	}

	ct, err := detectContentType(r)
	if err != nil {
		return "", fmt.Errorf("update avatar: detect content type: %w", err)
	}

	if ct != "image/png" && ct != "image/jpeg" {
		return "", ErrUnsupportedAvatarFormat
	}

	img, err := imaging.Decode(io.LimitReader(r, MaxAvatarBytes), imaging.AutoOrientation(true))
	if err == image.ErrFormat {
		return "", ErrUnsupportedAvatarFormat
	}

	if err != nil {
		return "", fmt.Errorf("could not read avatar: %w", err)
	}

	buf := &bytes.Buffer{}
	img = imaging.Fill(img, 400, 400, imaging.Center, imaging.CatmullRom)
	if ct == "image/png" {
		err = png.Encode(buf, img)
	} else {
		err = jpeg.Encode(buf, img, nil)
	}
	if err != nil {
		return "", fmt.Errorf("could not resize avatar: %w", err)
	}

	avatarFileName, err := gonanoid.New()
	if err != nil {
		return "", fmt.Errorf("could not generate avatar filename: %w", err)
	}

	if ct == "image/png" {
		avatarFileName += ".png"
	} else {
		avatarFileName += ".jpg"
	}

	err = s.Store.Store(ctx, AvatarsBucket, avatarFileName, buf.Bytes(), storage.StoreWithContentType(ct))
	if err != nil {
		return "", fmt.Errorf("could not store avatar file: %w", err)
	}

	oldAvatar, err := s.Cockroach.UpdateAvatar(ctx, uid, avatarFileName)
	if err != nil {
		defer func() {
			err := s.Store.Delete(context.Background(), AvatarsBucket, avatarFileName)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not delete avatar file after user update fail: %w", err))
			}
		}()

		return "", err
	}

	if oldAvatar != nil {
		defer func() {
			err := s.Store.Delete(context.Background(), AvatarsBucket, *oldAvatar)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not delete old avatar: %w", err))
			}
		}()
	}

	return s.AvatarURLPrefix + avatarFileName, nil
}

// UpdateCover of the authenticated user returning the new cover URL.
// Please limit the reader before hand using MaxCoverBytes.
func (s *Service) UpdateCover(ctx context.Context, r io.ReadSeeker) (string, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return "", errs.Unauthenticated
	}

	ct, err := detectContentType(r)
	if err != nil {
		return "", fmt.Errorf("update cover: detect content type: %w", err)
	}

	if ct != "image/png" && ct != "image/jpeg" {
		return "", ErrUnsupportedAvatarFormat
	}

	img, err := imaging.Decode(io.LimitReader(r, MaxCoverBytes), imaging.AutoOrientation(true))
	if err == image.ErrFormat {
		return "", ErrUnsupportedCoverFormat
	}

	if err != nil {
		return "", fmt.Errorf("could not read cover: %w", err)
	}

	buf := &bytes.Buffer{}
	img = imaging.CropCenter(img, 2560, 423)
	if ct == "image/png" {
		err = png.Encode(buf, img)
	} else {
		err = jpeg.Encode(buf, img, nil)
	}
	if err != nil {
		return "", fmt.Errorf("could not resize cover: %w", err)
	}

	coverFileName, err := gonanoid.New()
	if err != nil {
		return "", fmt.Errorf("could not generate cover filename: %w", err)
	}

	if ct == "image/png" {
		coverFileName += ".png"
	} else {
		coverFileName += ".jpg"
	}

	err = s.Store.Store(ctx, CoversBucket, coverFileName, buf.Bytes(), storage.StoreWithContentType(ct))
	if err != nil {
		return "", fmt.Errorf("could not store cover file: %w", err)
	}

	var oldCover sql.NullString
	query := `
		UPDATE users SET cover = $1 WHERE id = $2
		RETURNING (SELECT cover FROM users WHERE id = $2) AS old_cover
	`
	row := s.DB.QueryRow(ctx, query, coverFileName, uid)
	err = row.Scan(&oldCover)
	if err != nil {
		defer func() {
			err := s.Store.Delete(context.Background(), CoversBucket, coverFileName)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not delete cover file after user update fail: %w", err))
			}
		}()

		return "", fmt.Errorf("could not update cover: %w", err)
	}

	if oldCover.Valid {
		defer func() {
			err := s.Store.Delete(context.Background(), CoversBucket, oldCover.String)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not delete old cover: %w", err))
			}
		}()
	}

	return s.CoverURLPrefix + coverFileName, nil
}

// ToggleFollow between two users.
func (s *Service) ToggleFollow(ctx context.Context, username string) (types.ToggleFollowOutput, error) {
	var out types.ToggleFollowOutput
	followerID, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, errs.Unauthenticated
	}

	username = strings.TrimSpace(username)
	if !types.ValidUsername(username) {
		return out, ErrInvalidUsername
	}

	var followeeID string
	err := cockroach.ExecuteTx(ctx, s.DB, func(tx pgx.Tx) error {
		query := "SELECT id FROM users WHERE username = $1"
		err := tx.QueryRow(ctx, query, username).Scan(&followeeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}

		if err != nil {
			return fmt.Errorf("could not query select user id from username: %w", err)
		}

		if followeeID == followerID {
			return ErrForbiddenFollow
		}

		query = `
			SELECT EXISTS (
				SELECT 1 FROM follows WHERE follower_id = $1 AND followee_id = $2
			)`
		row := tx.QueryRow(ctx, query, followerID, followeeID)
		err = row.Scan(&out.Following)
		if err != nil {
			return fmt.Errorf("could not query select existence of follow: %w", err)
		}

		if out.Following {
			query = "DELETE FROM follows WHERE follower_id = $1 AND followee_id = $2"
			_, err = tx.Exec(ctx, query, followerID, followeeID)
			if err != nil {
				return fmt.Errorf("could not delete follow: %w", err)
			}

			query = "UPDATE users SET followees_count = followees_count - 1 WHERE id = $1"
			if _, err = tx.Exec(ctx, query, followerID); err != nil {
				return fmt.Errorf("could not decrement followees count: %w", err)
			}

			query = `
				UPDATE users SET followers_count = followers_count - 1 WHERE id = $1
				RETURNING followers_count`
			if err = tx.QueryRow(ctx, query, followeeID).
				Scan(&out.FollowersCount); err != nil {
				return fmt.Errorf("could not decrement followers count: %w", err)
			}
		} else {
			query = "INSERT INTO follows (follower_id, followee_id) VALUES ($1, $2)"
			_, err = tx.Exec(ctx, query, followerID, followeeID)
			if err != nil {
				return fmt.Errorf("could not insert follow: %w", err)
			}

			query = "UPDATE users SET followees_count = followees_count + 1 WHERE id = $1"
			if _, err = tx.Exec(ctx, query, followerID); err != nil {
				return fmt.Errorf("could not increment followees count: %w", err)
			}

			query = `
				UPDATE users SET followers_count = followers_count + 1 WHERE id = $1
				RETURNING followers_count`
			row = tx.QueryRow(ctx, query, followeeID)
			err = row.Scan(&out.FollowersCount)
			if err != nil {
				return fmt.Errorf("could not increment followers count: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return out, err
	}

	out.Following = !out.Following

	if out.Following {
		go s.notifyFollow(followerID, followeeID)
	}

	return out, nil
}

// Followers in ascending order with forward pagination.
func (s *Service) Followers(ctx context.Context, username string, first uint64, after *string) (types.UserProfiles, error) {
	username = strings.TrimSpace(username)
	if !types.ValidUsername(username) {
		return nil, ErrInvalidUsername
	}

	var afterUsername string
	if after != nil {
		var err error
		afterUsername, err = cursor.DecodeSimple(*after)
		if err != nil || !types.ValidUsername(afterUsername) {
			return nil, ErrInvalidCursor
		}
	}

	first = normalizePageSize(first)
	uid, auth := ctx.Value(KeyAuthUserID).(string)
	query, args, err := buildQuery(`
		SELECT users.id
		, users.email
		, users.username
		, users.avatar
		, users.cover
		, users.followers_count
		, users.followees_count
		{{ if .auth }}
		, followers.follower_id IS NOT NULL AS following
		, followees.followee_id IS NOT NULL AS followeed
		{{ end }}
		FROM follows
		INNER JOIN users ON follows.follower_id = users.id
		{{ if .auth }}
		LEFT JOIN follows AS followers
			ON followers.follower_id = @uid AND followers.followee_id = users.id
		LEFT JOIN follows AS followees
			ON followees.follower_id = users.id AND followees.followee_id = @uid
		{{ end }}
		WHERE follows.followee_id = (SELECT id FROM users WHERE username = @username)
		{{ if .afterUsername }}AND username > @afterUsername{{ end }}
		ORDER BY username ASC
		LIMIT @first`, map[string]any{
		"auth":          auth,
		"uid":           uid,
		"username":      username,
		"first":         first,
		"afterUsername": afterUsername,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build followers sql query: %w", err)
	}

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select followers: %w", err)
	}

	defer rows.Close()

	var uu types.UserProfiles
	for rows.Next() {
		var u types.UserProfile
		var avatar, cover sql.NullString
		dest := []any{
			&u.ID,
			&u.Email,
			&u.Username,
			&avatar,
			&cover,
			&u.FollowersCount,
			&u.FolloweesCount,
		}
		if auth {
			dest = append(dest, &u.Following, &u.Followeed)
		}
		if err = rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("could not scan follower: %w", err)
		}

		u.Me = auth && uid == u.ID
		if !u.Me {
			u.ID = ""
			u.Email = ""
		}
		u.AvatarURL = s.avatarURL(avatar)
		u.CoverURL = s.coverURL(cover)
		uu = append(uu, u)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not iterate follower rows: %w", err)
	}

	return uu, nil
}

// Followees in ascending order with forward pagination.
func (s *Service) Followees(ctx context.Context, username string, first uint64, after *string) (types.UserProfiles, error) {
	username = strings.TrimSpace(username)
	if !types.ValidUsername(username) {
		return nil, ErrInvalidUsername
	}

	var afterUsername string
	if after != nil {
		var err error
		afterUsername, err = cursor.DecodeSimple(*after)
		if err != nil || !types.ValidUsername(afterUsername) {
			return nil, ErrInvalidCursor
		}
	}

	first = normalizePageSize(first)
	uid, auth := ctx.Value(KeyAuthUserID).(string)
	query, args, err := buildQuery(`
		SELECT users.id
		, users.email
		, users.username
		, users.avatar
		, users.cover
		, users.followers_count
		, users.followees_count
		{{ if .auth }}
		, followers.follower_id IS NOT NULL AS following
		, followees.followee_id IS NOT NULL AS followeed
		{{ end }}
		FROM follows
		INNER JOIN users ON follows.followee_id = users.id
		{{ if .auth }}
		LEFT JOIN follows AS followers
			ON followers.follower_id = @uid AND followers.followee_id = users.id
		LEFT JOIN follows AS followees
			ON followees.follower_id = users.id AND followees.followee_id = @uid
		{{ end }}
		WHERE follows.follower_id = (SELECT id FROM users WHERE username = @username)
		{{ if .afterUsername }}AND username > @afterUsername{{ end }}
		ORDER BY username ASC
		LIMIT @first`, map[string]any{
		"auth":          auth,
		"uid":           uid,
		"username":      username,
		"first":         first,
		"afterUsername": afterUsername,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build followees sql query: %w", err)
	}

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select followees: %w", err)
	}

	defer rows.Close()

	var uu types.UserProfiles
	for rows.Next() {
		var u types.UserProfile
		var avatar, cover sql.NullString
		dest := []any{
			&u.ID,
			&u.Email,
			&u.Username,
			&avatar,
			&cover,
			&u.FollowersCount,
			&u.FolloweesCount,
		}
		if auth {
			dest = append(dest, &u.Following, &u.Followeed)
		}
		if err = rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("could not scan followee: %w", err)
		}

		u.Me = auth && uid == u.ID
		if !u.Me {
			u.ID = ""
			u.Email = ""
		}
		u.AvatarURL = s.avatarURL(avatar)
		u.CoverURL = s.coverURL(cover)
		uu = append(uu, u)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not iterate followee rows: %w", err)
	}

	return uu, nil
}

func (s *Service) avatarURL(avatar sql.NullString) *string {
	if !avatar.Valid {
		return nil
	}
	str := s.AvatarURLPrefix + avatar.String
	return &str
}

func (s *Service) coverURL(cover sql.NullString) *string {
	if !cover.Valid {
		return nil
	}

	str := s.CoverURLPrefix + cover.String
	return &str
}
