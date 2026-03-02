package service

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"time"

	"github.com/nakamauwu/nakama/textutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

const (
	postContentMaxLength = 2048
	postSpoilerMaxLength = 64
	postEditWindow       = time.Minute * 15
)

var (
	// ErrInvalidCursor denotes an invalid cursor, that is not base64 encoded and has a key and timestamp separated by comma.
	ErrInvalidCursor = errs.InvalidArgumentError("invalid cursor")
)

func (s *Service) Posts(ctx context.Context, in types.ListPosts) (types.Page[types.Post], error) {
	var out types.Page[types.Post]

	if err := in.Validate(); err != nil {
		return out, err
	}

	if uid, ok := ctx.Value(KeyAuthUserID).(string); ok {
		in.SetViewerID(uid)
	}

	out, err := s.Cockroach.Posts(ctx, in)
	if err != nil {
		return out, err
	}

	for i, p := range out.Items {
		if p.User != nil {
			p.User.SetAvatarURL(s.AvatarURLPrefix)
		}
		p.SetMediaURLs(s.MediaURLPrefix)
		out.Items[i] = p
	}

	return out, nil
}

// PostStream to receive posts in realtime.
func (s *Service) PostStream(ctx context.Context) (<-chan types.Post, error) {
	pp := make(chan types.Post)
	unsub, err := s.PubSub.Sub(postsTopic, func(data []byte) {
		go func(r io.Reader) {
			var p types.Post
			err := gob.NewDecoder(r).Decode(&p)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not gob decode post: %w", err))
				return
			}

			pp <- p
		}(bytes.NewReader(data))
	})
	if err != nil {
		return nil, fmt.Errorf("could not subscribe to posts: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := unsub(); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not unsubcribe from posts: %w", err))
			// don't return
		}

		close(pp)
	}()

	return pp, nil
}

// Post with the given ID.
func (s *Service) Post(ctx context.Context, postID string) (types.Post, error) {
	var out types.Post
	if !types.ValidUUIDv4(postID) {
		return out, errs.InvalidArgumentError("invalid post ID")
	}

	in := types.RetrievePost{PostID: postID}

	if uid, ok := ctx.Value(KeyAuthUserID).(string); ok {
		in.SetViewerID(uid)
	}

	post, err := s.Cockroach.Post(ctx, in)
	if err != nil {
		return out, err
	}

	if post.User != nil {
		post.User.SetAvatarURL(s.AvatarURLPrefix)
	}
	post.SetMediaURLs(s.MediaURLPrefix)

	return post, nil
}

func (s *Service) UpdatePost(ctx context.Context, in types.UpdatePost) (types.UpdatedPost, error) {
	var out types.UpdatedPost

	if err := in.Validate(); err != nil {
		return out, err
	}

	if err := s.authorize(ctx, ResourceKindPost, in.ID); err != nil {
		return out, err
	}

	if in.Content != nil {
		in.SetTags(textutil.CollectTags(*in.Content))
	}

	postCreatedAt, err := s.Cockroach.PostCreatedAt(ctx, in.ID)
	if err != nil {
		return out, err
	}

	if time.Since(postCreatedAt) > postEditWindow {
		return out, errs.PermissionDeniedError("update post denied")
	}

	// TODO: recollect mentions

	return s.Cockroach.UpdatePost(ctx, in)
}

func (s *Service) DeletePost(ctx context.Context, postID string) error {
	if !types.ValidUUIDv4(postID) {
		return errs.InvalidArgumentError("invalid post ID")
	}

	if err := s.authorize(ctx, ResourceKindPost, postID); err != nil {
		return err
	}

	return s.Cockroach.DeletePost(ctx, postID)
}

func (s *Service) TogglePostReaction(ctx context.Context, in types.TogglePostReaction) ([]types.Reaction, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, errs.Unauthenticated
	}

	in.SetUserID(uid)

	return s.Cockroach.TogglePostReaction(ctx, in)
}

// TogglePostSubscription so you can stop receiving notifications from a thread.
func (s *Service) TogglePostSubscription(ctx context.Context, postID string) (types.ToggledSubscription, error) {
	var out types.ToggledSubscription

	if !types.ValidUUIDv4(postID) {
		return out, errs.InvalidArgumentError("invalid post ID")
	}

	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, errs.Unauthenticated
	}

	in := types.ToggleSubscription{PostID: postID}
	in.SetUserID(uid)

	return s.Cockroach.ToggleSubscription(ctx, in)
}

const postsTopic = "posts"

func (s *Service) broadcastPost(p types.Post) {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(p)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not gob encode post: %w", err))
		return
	}

	err = s.PubSub.Pub(postsTopic, b.Bytes())
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not publish post: %w", err))
		return
	}
}
