package service

import (
	"context"
	"fmt"
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

	if err := processPostAttachments(ctx, &in); err != nil {
		return out, fmt.Errorf("process attachments: %w", err)
	}

	cleanup, err := svc.Minio.UploadMany(ctx, "post-attachments", in.ProcessedAttachments())
	if err != nil {
		return out, err
	}

	out, err = svc.Cockroach.CreatePost(ctx, in)
	if err != nil {
		go cleanup()
		return out, err
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

func processPostAttachments(ctx context.Context, in *types.CreatePost) error {
	images, err := ffmpeg.ResizeImages(ctx, 2_000, in.Attachments)
	if err != nil {
		return err
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

	in.SetProcessedAttachments(processed)

	return nil
}
