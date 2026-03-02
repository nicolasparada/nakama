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

const commentEditWindow = time.Minute * 15

// CreateComment on a post.
func (s *Service) CreateComment(ctx context.Context, in types.CreateComment) (types.Comment, error) {
	var c types.Comment
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return c, errs.Unauthenticated
	}

	if err := in.Validate(); err != nil {
		return c, err
	}

	in.SetUserID(uid)
	in.SetTags(textutil.CollectTags(in.Content))

	created, err := s.Cockroach.CreateComment(ctx, in)
	if err != nil {
		return c, err
	}

	c.ID = created.ID
	c.CreatedAt = created.CreatedAt

	c.UserID = uid
	c.PostID = in.PostID
	c.Content = in.Content
	c.Mine = true

	go s.commentCreated(c)

	return c, nil
}

func (s *Service) commentCreated(c types.Comment) {
	u, err := s.userByID(context.Background(), c.UserID)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not fetch comment user: %w", err))
		return
	}

	c.User = &u
	c.Mine = false

	go s.notifyComment(c)
	go s.notifyCommentMention(c)
	go s.broadcastComment(c)
}

// Comments from a post in descending order with backward pagination.
func (s *Service) Comments(ctx context.Context, in types.ListComments) (types.Page[types.Comment], error) {
	var out types.Page[types.Comment]

	if err := in.Validate(); err != nil {
		return out, err
	}

	if userID, ok := ctx.Value(KeyAuthUserID).(string); ok {
		in.SetViewerID(userID)
	}

	out, err := s.Cockroach.Comments(ctx, in)
	if err != nil {
		return out, err
	}

	for i, c := range out.Items {
		if c.User == nil {
			continue
		}

		c.User.SetAvatarURL(s.AvatarURLPrefix)
		out.Items[i] = c
	}

	return out, nil
}

// CommentStream to receive comments in realtime.
func (s *Service) CommentStream(ctx context.Context, postID string) (<-chan types.Comment, error) {
	if !types.ValidUUIDv4(postID) {
		return nil, errs.InvalidArgumentError("invalid post ID")
	}

	cc := make(chan types.Comment)
	uid, auth := ctx.Value(KeyAuthUserID).(string)
	unsub, err := s.PubSub.Sub(commentTopic(postID), func(data []byte) {
		go func(r io.Reader) {
			var c types.Comment
			err := gob.NewDecoder(r).Decode(&c)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not gob decode comment: %w", err))
				return
			}

			if auth && uid == c.UserID {
				return
			}

			cc <- c
		}(bytes.NewReader(data))
	})
	if err != nil {
		return nil, fmt.Errorf("could not subscribe to comments: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := unsub(); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not unsubcribe from comments: %w", err))
			// don't return
		}
		close(cc)
	}()

	return cc, nil
}

func (s *Service) UpdateComment(ctx context.Context, in types.UpdateComment) (types.UpdatedComment, error) {
	var out types.UpdatedComment

	if err := in.Validate(); err != nil {
		return out, err
	}

	if err := s.authorize(ctx, ResourceKindComment, in.ID); err != nil {
		return out, err
	}

	if in.Content != nil {
		in.SetTags(textutil.CollectTags(*in.Content))
	}

	commentCreatedAt, err := s.Cockroach.CommentCreatedAt(ctx, in.ID)
	if err != nil {
		return out, err
	}

	if time.Since(commentCreatedAt) > commentEditWindow {
		return out, errs.PermissionDeniedError("update comment denied")
	}

	// TODO: recollect mentions

	return s.Cockroach.UpdateComment(ctx, in)
}

func (s *Service) DeleteComment(ctx context.Context, commentID string) error {
	if !types.ValidUUIDv4(commentID) {
		return errs.InvalidArgumentError("invalid comment ID")
	}

	if err := s.authorize(ctx, ResourceKindComment, commentID); err != nil {
		return err
	}

	return s.Cockroach.DeleteComment(ctx, commentID)
}

func (s *Service) ToggleCommentReaction(ctx context.Context, in types.ToggleCommentReaction) ([]types.Reaction, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, errs.Unauthenticated
	}

	in.SetUserID(uid)

	return s.Cockroach.ToggleCommentReaction(ctx, in)
}

func (s *Service) broadcastComment(c types.Comment) {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(c)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not gob encode comment: %w", err))
		return
	}

	err = s.PubSub.Pub(commentTopic(c.PostID), b.Bytes())
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not publish comment: %w", err))
		return
	}
}

func commentTopic(postID string) string { return "comment_" + postID }
