package service

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/ffmpeg"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
	"mvdan.cc/xurls/v2"
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

	images, err := ffmpeg.ResizeImages(ctx, 2_000, in.Attachments)
	if err != nil {
		return out, fmt.Errorf("resize images: %w", err)
	}

	in.SetProcessedAttachments(newAttachmentList(images))

	cleanup, err := svc.Minio.UploadMany(ctx, "post-attachments", in.ProcessedAttachments())
	if err != nil {
		return out, err
	}

	out, err = svc.Cockroach.CreatePost(ctx, in)
	if err != nil {
		go cleanup()
		return out, err
	}

	// cache early
	_ = svc.Preview.Fetch(svc.baseCtx, extractURLs(in.Content))

	svc.background(func(ctx context.Context) error {
		return svc.Cockroach.FanoutPost(ctx, types.FanoutPost{
			UserID: in.UserID(),
			PostID: out.ID,
		})
	})

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

func (svc *Service) Feed(ctx context.Context, in types.ListFeed) (types.Page[types.Post], error) {
	var out types.Page[types.Post]

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetUserID(loggedInUser.ID)

	out, err := svc.Cockroach.Feed(ctx, in)
	if err != nil {
		return out, err
	}

	svc.setPostsPreviews(ctx, out)

	return out, nil
}

func (svc *Service) Posts(ctx context.Context, in types.ListPosts) (types.Page[types.Post], error) {
	if u, loggedIn := auth.UserFromContext(ctx); loggedIn {
		in.SetLoggedInUserID(u.ID)
	}

	out, err := svc.Cockroach.Posts(ctx, in)
	if err != nil {
		return out, err
	}

	svc.setPostsPreviews(ctx, out)

	return out, nil
}

func (svc *Service) Post(ctx context.Context, in types.RetrievePost) (types.Post, error) {
	var out types.Post

	if err := in.Validate(); err != nil {
		return out, err
	}

	out, err := svc.Cockroach.Post(ctx, in)
	if err != nil {
		return out, err
	}

	svc.setPostPreviews(ctx, &out)

	return out, nil
}

func (svc *Service) SearchPosts(ctx context.Context, in types.SearchPosts) (types.Page[types.Post], error) {
	return svc.Cockroach.SearchPosts(ctx, in)
}

func (svc *Service) ToggleReaction(ctx context.Context, in types.ToggleReaction) error {
	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	_, err := svc.Cockroach.ToggleReaction(ctx, in)
	return err
}

func newAttachmentList(images []ffmpeg.Image) []types.Attachment {
	now := time.Now()
	id := id.Generate()

	var attachments []types.Attachment
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
		attachments = append(attachments, attachment)
	}

	return attachments
}

func newAttachment(image ffmpeg.Image) types.Attachment {
	now := time.Now()
	id := id.Generate()

	path := fmt.Sprintf("%d/%d/%d/%d_%s.%s", now.Year(), now.Month(), now.Day(), now.Unix(), id, ffmpeg.ContentTypeToExtension[image.ContentType])
	attachment := types.Attachment{
		Path:        path,
		ContentType: image.ContentType,
		FileSize:    image.FileSize,
		Width:       image.Width,
		Height:      image.Height,
	}
	attachment.SetReader(image.File)
	return attachment
}

func (svc *Service) setPostsPreviews(ctx context.Context, posts types.Page[types.Post]) {
	for i, post := range posts.Items {
		results := svc.Preview.Fetch(ctx, extractURLs(post.Content))
		posts.Items[i].SetPreviews(results)
	}
}

func (svc *Service) setPostPreviews(ctx context.Context, post *types.Post) {
	results := svc.Preview.Fetch(ctx, extractURLs(post.Content))
	post.SetPreviews(results)
}

var reURL = xurls.Strict()

func extractURLs(text string) []string {
	var out []string
	for _, u := range reURL.FindAllString(text, -1) {
		if u, err := url.Parse(u); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
			out = append(out, u.String())
		}
	}
	return out
}
