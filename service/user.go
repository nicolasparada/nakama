package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"github.com/disintegration/imaging"
	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/nakamauwu/nakama/storage"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

const (
	MaxAvatarBytes = 5 << 20  // 5MB
	MaxCoverBytes  = 20 << 20 // 20MB

	AvatarsBucket = "avatars"
	CoversBucket  = "covers"
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
		return "", errs.InvalidArgumentError("unsupported avatar format")
	}

	img, err := imaging.Decode(io.LimitReader(r, MaxAvatarBytes), imaging.AutoOrientation(true))
	if err == image.ErrFormat {
		return "", errs.InvalidArgumentError("unsupported avatar format")
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
		return "", errs.InvalidArgumentError("unsupported cover format")
	}

	img, err := imaging.Decode(io.LimitReader(r, MaxCoverBytes), imaging.AutoOrientation(true))
	if err == image.ErrFormat {
		return "", errs.InvalidArgumentError("unsupported cover format")
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

	oldCover, err := s.Cockroach.UpdateCover(ctx, uid, coverFileName)
	if err != nil {
		defer func() {
			err := s.Store.Delete(context.Background(), CoversBucket, coverFileName)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not delete cover file after user update fail: %w", err))
			}
		}()

		return "", fmt.Errorf("could not update cover: %w", err)
	}

	if oldCover != nil {
		defer func() {
			err := s.Store.Delete(context.Background(), CoversBucket, *oldCover)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not delete old cover: %w", err))
			}
		}()
	}

	return s.CoverURLPrefix + coverFileName, nil
}

func (s *Service) ToggleFollow(ctx context.Context, username string) (types.ToggledFollow, error) {
	var out types.ToggledFollow

	username = strings.TrimSpace(username)
	if !types.ValidUsername(username) {
		return out, errs.InvalidArgumentError("invalid username")
	}

	followerID, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, errs.Unauthenticated
	}

	followeeID, err := s.Cockroach.UserIDFromUsername(ctx, username)
	if err != nil {
		return out, err
	}

	if followerID == followeeID {
		return out, errs.PermissionDeniedError("forbidden follow")
	}

	out, err = s.Cockroach.ToggleFollow(ctx, followerID, followeeID)
	if err != nil {
		return out, err
	}

	if out.FollowedByViewer {
		go s.notifyFollow(followerID, followeeID)
	}

	return out, nil
}

func (s *Service) Followers(ctx context.Context, in types.ListFollowers) (types.Page[types.UserProfile], error) {
	var out types.Page[types.UserProfile]

	if err := in.Validate(); err != nil {
		return out, err
	}

	if uid, ok := ctx.Value(KeyAuthUserID).(string); ok {
		in.SetViewerID(uid)
	}

	out, err := s.Cockroach.Followers(ctx, in)
	if err != nil {
		return out, err
	}

	for i, u := range out.Items {
		u.SetAvatarURL(s.AvatarURLPrefix)
		u.SetCoverURL(s.CoverURLPrefix)
		out.Items[i] = u
	}

	return out, nil
}

func (s *Service) Followees(ctx context.Context, in types.ListFollowees) (types.Page[types.UserProfile], error) {
	var out types.Page[types.UserProfile]

	if err := in.Validate(); err != nil {
		return out, err
	}

	if uid, ok := ctx.Value(KeyAuthUserID).(string); ok {
		in.SetViewerID(uid)
	}

	out, err := s.Cockroach.Followees(ctx, in)
	if err != nil {
		return out, err
	}

	for i, u := range out.Items {
		u.SetAvatarURL(s.AvatarURLPrefix)
		u.SetCoverURL(s.CoverURLPrefix)
		out.Items[i] = u
	}

	return out, nil
}
