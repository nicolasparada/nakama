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
	"unicode/utf8"

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

var (
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

// Users in ascending order with forward pagination and filtered by username.
func (s *Service) Users(ctx context.Context, search string, first uint64, after *string) (types.UserProfiles, error) {
	search = strings.TrimSpace(search)
	first = normalizePageSize(first)

	var afterUsername string
	if after != nil {
		var err error
		afterUsername, err = cursor.DecodeSimple(*after)
		if err != nil || !types.ValidUsername(afterUsername) {
			return nil, ErrInvalidCursor
		}
	}

	uid, auth := ctx.Value(KeyAuthUserID).(string)
	query, args, err := buildQuery(`
		SELECT id, email, username, avatar, cover, bio, waifu, husbando, followers_count, followees_count
		{{ if .auth }}
		, followers.follower_id IS NOT NULL AS following
		, followees.followee_id IS NOT NULL AS followeed
		{{ end }}
		FROM users
		{{ if .auth }}
		LEFT JOIN follows AS followers
			ON followers.follower_id = @uid AND followers.followee_id = users.id
		LEFT JOIN follows AS followees
			ON followees.follower_id = users.id AND followees.followee_id = @uid
		{{ end }}
		{{ if or .search .afterUsername }}WHERE{{ end }}
		{{ if .search }}username ILIKE '%' || @search || '%'{{ end }}
		{{ if and .search .afterUsername }}AND{{ end }}
		{{ if .afterUsername }}username > @afterUsername{{ end }}
		ORDER BY username ASC
		LIMIT @first`, map[string]any{
		"auth":          auth,
		"uid":           uid,
		"search":        search,
		"first":         first,
		"afterUsername": afterUsername,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build users sql query: %w", err)
	}

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select users: %w", err)
	}

	defer rows.Close()

	var uu types.UserProfiles
	for rows.Next() {
		var u types.UserProfile
		var avatar, cover sql.NullString
		dest := []any{
			&u.ID, &u.Email,
			&u.Username,
			&avatar,
			&cover,
			&u.Bio,
			&u.Waifu,
			&u.Husbando,
			&u.FollowersCount,
			&u.FolloweesCount,
		}
		if auth {
			dest = append(dest, &u.Following, &u.Followeed)
		}
		if err = rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("could not scan user: %w", err)
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
		return nil, fmt.Errorf("could not iterate user rows: %w", err)
	}

	return uu, nil
}

// Usernames to autocomplete a mention box or something.
func (s *Service) Usernames(ctx context.Context, startingWith string, first uint64, after *string) (types.Usernames, error) {
	startingWith = strings.TrimSpace(startingWith)
	if startingWith == "" {
		return nil, nil
	}

	var afterUsername string
	if after != nil {
		var err error
		afterUsername, err = cursor.DecodeSimple(*after)
		if err != nil || !types.ValidUsername(afterUsername) {
			return nil, ErrInvalidCursor
		}
	}

	uid, auth := ctx.Value(KeyAuthUserID).(string)
	first = normalizePageSize(first)
	query, args, err := buildQuery(`
		SELECT username FROM users
		WHERE username ILIKE @startingWith || '%'
		{{ if .auth }}AND users.id != @uid{{ end }}
		{{ if .afterUsername }}AND username > @afterUsername{{ end }}
		ORDER BY username ASC
		LIMIT @first`, map[string]any{
		"startingWith":  startingWith,
		"auth":          auth,
		"uid":           uid,
		"first":         first,
		"afterUsername": afterUsername,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build usernames sql query: %w", err)
	}

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select usernames: %w", err)
	}

	defer rows.Close()

	var uu types.Usernames
	for rows.Next() {
		var u string
		if err = rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("could not scan username: %w", err)
		}

		uu = append(uu, u)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not iterate username rows: %w", err)
	}

	return uu, nil
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

// User with the given username.
func (s *Service) User(ctx context.Context, username string) (types.UserProfile, error) {
	var u types.UserProfile

	username = strings.TrimSpace(username)
	if !types.ValidUsername(username) {
		return u, ErrInvalidUsername
	}

	uid, auth := ctx.Value(KeyAuthUserID).(string)
	query, args, err := buildQuery(`
		SELECT id, email, avatar, cover, bio, waifu, husbando, followers_count, followees_count
		{{if .auth}}
		, followers.follower_id IS NOT NULL AS following
		, followees.followee_id IS NOT NULL AS followeed
		{{end}}
		FROM users
		{{if .auth}}
		LEFT JOIN follows AS followers
			ON followers.follower_id = @uid AND followers.followee_id = users.id
		LEFT JOIN follows AS followees
			ON followees.follower_id = users.id AND followees.followee_id = @uid
		{{end}}
		WHERE username = @username`, map[string]any{
		"auth":     auth,
		"uid":      uid,
		"username": username,
	})
	if err != nil {
		return u, fmt.Errorf("could not build user sql query: %w", err)
	}

	var avatar, cover sql.NullString
	dest := []any{&u.ID, &u.Email, &avatar, &cover, &u.Bio, &u.Waifu, &u.Husbando, &u.FollowersCount, &u.FolloweesCount}
	if auth {
		dest = append(dest, &u.Following, &u.Followeed)
	}
	err = s.DB.QueryRow(ctx, query, args...).Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrUserNotFound
	}

	if err != nil {
		return u, fmt.Errorf("could not query select user: %w", err)
	}

	u.Username = username
	u.Me = auth && uid == u.ID
	if !u.Me {
		u.ID = ""
		u.Email = ""
	}
	u.AvatarURL = s.avatarURL(avatar)
	u.CoverURL = s.coverURL(cover)
	return u, nil
}

func (s *Service) UpdateUser(ctx context.Context, params types.UpdateUserParams) error {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return errs.Unauthenticated
	}

	if params.Username != nil {
		*params.Username = strings.TrimSpace(*params.Username)
		if !types.ValidUsername(*params.Username) {
			return ErrInvalidUsername
		}
	}

	if params.Bio != nil {
		*params.Bio = strings.TrimSpace(*params.Bio)
		if !validUserBio(*params.Bio) {
			return ErrInvalidUserBio
		}
	}

	if params.Waifu != nil {
		*params.Waifu = strings.TrimSpace(*params.Waifu)
		if !validAnimeCharName(*params.Waifu) {
			return ErrInvalidUserWaifu
		}
	}

	if params.Husbando != nil {
		*params.Husbando = strings.TrimSpace(*params.Husbando)
		if !validAnimeCharName(*params.Husbando) {
			return ErrInvalidUserHusbando
		}
	}

	query := `
		UPDATE users SET
			username = COALESCE($1, username)
			, bio = $2
			, waifu = $3
			, husbando = $4
		WHERE id = $5`
	_, err := s.DB.Exec(ctx, query, params.Username, params.Bio, params.Waifu, params.Husbando, uid)
	if isUniqueViolation(err) {
		return ErrUsernameTaken
	}

	if err != nil {
		return fmt.Errorf("could not sql update user: %w", err)
	}

	return nil
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

	var oldAvatar sql.NullString
	query := `
		UPDATE users SET avatar = $1 WHERE id = $2
		RETURNING (SELECT avatar FROM users WHERE id = $2) AS old_avatar
	`
	row := s.DB.QueryRow(ctx, query, avatarFileName, uid)
	err = row.Scan(&oldAvatar)
	if err != nil {
		defer func() {
			err := s.Store.Delete(context.Background(), AvatarsBucket, avatarFileName)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not delete avatar file after user update fail: %w", err))
			}
		}()

		return "", fmt.Errorf("could not update avatar: %w", err)
	}

	if oldAvatar.Valid {
		defer func() {
			err := s.Store.Delete(context.Background(), AvatarsBucket, oldAvatar.String)
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

func validUserBio(s string) bool {
	return s != "" && utf8.RuneCountInString(s) <= 480
}

func validAnimeCharName(s string) bool {
	return s != "" && utf8.RuneCountInString(s) <= 32
}
