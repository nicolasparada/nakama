package service

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/ffmpeg"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) CreatePost(ctx context.Context, in types.CreatePost) (types.Created, error) {
	var out types.Created

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetUserID(loggedInUser.ID)

	processedAttachments, err := processAttachments(ctx, 2_000, in.Attachments)
	if err != nil {
		return out, fmt.Errorf("process attachments: %w", err)
	}

	in.SetProcessedAttachments(processedAttachments)

	cleanup, err := svc.Minio.UploadMany(ctx, "post-attachments", processedAttachments)
	if err != nil {
		return out, err
	}

	out, err = svc.Cockroach.CreatePost(ctx, in)
	if err != nil {
		go cleanup()
		return out, err
	}

	if mentions := extractMentions(in.Content); len(mentions) != 0 {
		svc.background(func(ctx context.Context) error {
			return svc.Cockroach.CreateMentionNotifications(ctx, types.CreateMentionNotifications{
				ActorUserID:    loggedInUser.ID,
				NotifiableKind: types.NotifiableKindPost,
				NotifiableID:   out.ID,
				Usernames:      mentions,
			})
		})
	}

	return out, nil
}

func (svc *Service) Posts(ctx context.Context) (types.Page[types.Post], error) {
	return svc.Cockroach.Posts(ctx)
}

func (svc *Service) Post(ctx context.Context, postID string) (types.Post, error) {
	var out types.Post

	if !id.Valid(postID) {
		return out, errs.NewInvalidArgumentError("PostID", "Invalid post ID")
	}

	return svc.Cockroach.Post(ctx, postID)
}

func processAttachments(ctx context.Context, maxRes uint32, attachments []io.ReadSeeker) ([]types.Attachment, error) {
	images, err := ffmpeg.ResizeImages(ctx, maxRes, attachments)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	id := id.Generate()

	processed := make([]types.Attachment, len(images))
	for i, img := range images {
		path := fmt.Sprintf("%d/%d/%d/%d_%s_%d.%s", now.Year(), now.Month(), now.Day(), now.Unix(), id, i, ffmpeg.ContentTypeToExtension[img.ContentType])
		attachment := types.Attachment{
			Path:        path,
			ContentType: img.ContentType,
			FileSize:    img.FileSize,
			Width:       img.Width,
			Height:      img.Height,
		}
		attachment.SetReader(img.File)
		processed[i] = attachment
	}

	return processed, nil
}
