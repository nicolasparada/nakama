package service

import (
	"context"
	"fmt"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/ffmpeg"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) CreateComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetUserID(loggedInUser.ID)

	var cleanupFile func()

	if in.File != nil {
		image, err := ffmpeg.ResizeImage(ctx, 2_000, in.File)
		if err != nil {
			return out, fmt.Errorf("resize image: %w", err)
		}

		in.SetAttachment(newAttachment(image))

		cleanupFile, err = svc.Minio.Upload(ctx, "comment-attachments", *in.Attachment())
		if err != nil {
			return out, err
		}
	}

	out, err := svc.Cockroach.CreateComment(ctx, in)
	if err != nil {
		if cleanupFile != nil {
			go cleanupFile()
		}
		return out, err
	}

	if mentions := extractMentions(in.Content); len(mentions) != 0 {
		svc.background(func(ctx context.Context) error {
			return svc.Cockroach.CreateMentionNotifications(ctx, types.CreateMentionNotifications{
				ActorUserID:    loggedInUser.ID,
				NotifiableKind: types.NotifiableKindComment,
				NotifiableID:   out.ID,
				Usernames:      mentions,
			})
		})
	}

	return out, nil
}

func (svc *Service) Comments(ctx context.Context, in types.ListComments) (types.Page[types.Comment], error) {
	var out types.Page[types.Comment]

	if err := in.Validate(); err != nil {
		return out, err
	}

	if u, loggedIn := auth.UserFromContext(ctx); loggedIn {
		in.SetLoggedInUserID(u.ID)
	}

	return svc.Cockroach.Comments(ctx, in)
}

func (svc *Service) ToggleCommentReaction(ctx context.Context, in types.ToggleCommentReaction) error {
	if err := in.Validate(); err != nil {
		return err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	_, err := svc.Cockroach.ToggleCommentReaction(ctx, in)
	return err
}
