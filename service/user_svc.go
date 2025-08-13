package service

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/ffmpeg"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) UpsertUser(ctx context.Context, in types.UpsertUser) (types.User, error) {
	var out types.User

	if err := in.Validate(); err != nil {
		return out, err
	}

	return svc.Cockroach.UpsertUser(ctx, in)
}

func (svc *Service) User(ctx context.Context, in types.RetrieveUser) (types.User, error) {
	var out types.User

	if err := in.Validate(); err != nil {
		return out, err
	}

	if u, loggedIn := auth.UserFromContext(ctx); loggedIn {
		in.SetLoggedInUserID(u.ID)
	}

	return svc.Cockroach.User(ctx, in)
}

func (svc *Service) UserFromUsername(ctx context.Context, in types.RetrieveUserFromUsername) (types.User, error) {
	var out types.User

	if err := in.Validate(); err != nil {
		return out, err
	}

	if u, loggedIn := auth.UserFromContext(ctx); loggedIn {
		in.SetLoggedInUserID(u.ID)
	}

	return svc.Cockroach.UserFromUsername(ctx, in)
}

func (svc *Service) ToggleFollow(ctx context.Context, in types.ToggleFollow) error {
	if err := in.Validate(); err != nil {
		return err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	if loggedInUser.ID == in.FolloweeID {
		return errs.NewPermissionDeniedError("Cannot follow yourself")
	}

	inserted, err := svc.Cockroach.ToggleFollow(ctx, in)
	if err != nil {
		return err
	}

	if inserted {
		svc.background(func(ctx context.Context) error {
			return svc.Cockroach.CreateFollowNotification(ctx, types.CreateFollowNotification{
				UserID:      in.FolloweeID,
				ActorUserID: loggedInUser.ID,
			})
		})
	}

	return nil
}

func (svc *Service) UploadAvatar(ctx context.Context, r io.ReadSeeker) error {
	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return errs.Unauthenticated
	}

	img, err := ffmpeg.ResizeImage(ctx, 400, r)
	if err != nil {
		return fmt.Errorf("resize avatar: %w", err)
	}

	attachment := newAttachment(img)

	cleanup, err := svc.Minio.Upload(ctx, "avatars", attachment)
	if err != nil {
		return err
	}

	err = svc.Cockroach.UpdateUserAvatar(ctx, types.UpdateUserAvatar{
		UserID: loggedInUser.ID,
		Avatar: attachment,
	})
	if err != nil {
		go cleanup()
		return err
	}

	return nil
}

var reMention = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_-])@([a-zA-Z0-9][a-zA-Z0-9_\.-]*)`)

func extractMentions(text string) []string {
	matches := reMention.FindAllStringSubmatch(text, -1)
	var mentions []string
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			username := match[1]
			// Clean up trailing punctuation that might be part of sentence structure
			username = cleanUsername(username)
			if username != "" && utf8.RuneCountInString(username) <= 21 && !seen[username] && !isPartOfEmail(text, match[0]) {
				mentions = append(mentions, username)
				seen[username] = true
			}
		}
	}

	return mentions
}

// cleanUsername removes trailing punctuation that's not part of the username
func cleanUsername(username string) string {
	// Remove trailing dots that are likely sentence punctuation
	for len(username) > 0 && username[len(username)-1] == '.' {
		username = username[:len(username)-1]
	}
	return username
}

// isPartOfEmail checks if the @username is part of an email address
func isPartOfEmail(text, fullMatch string) bool {
	// Find the position of the full match in the text
	pos := strings.Index(text, fullMatch)
	if pos == -1 {
		return false
	}

	// Check if there's another @ character immediately after the username
	endPos := pos + len(fullMatch)
	if endPos < len(text) && text[endPos] == '@' {
		return true
	}

	return false
}
