package service

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io"

	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

func (s *Service) Timeline(ctx context.Context, in types.ListTimeline) (types.Page[types.TimelineItem], error) {
	var out types.Page[types.TimelineItem]

	if err := in.Validate(); err != nil {
		return out, err
	}

	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, errs.Unauthenticated
	}

	in.SetUserID(uid)

	out, err := s.Cockroach.Timeline(ctx, in)
	if err != nil {
		return out, err
	}

	for i, ti := range out.Items {
		if ti.Post.User != nil {
			ti.Post.User.SetAvatarURL(s.MinioBaseURL, AvatarsBucket)
		}
		ti.Post.SetMediaURLs(s.MinioBaseURL, MediaBucket)
		out.Items[i] = ti
	}

	return out, nil
}

func (s *Service) TimelineItemStream(ctx context.Context) (<-chan types.TimelineItem, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, errs.Unauthenticated
	}

	tt := make(chan types.TimelineItem)
	unsub, err := s.PubSub.Sub(timelineTopic(uid), func(data []byte) {
		go func(r io.Reader) {
			var ti types.TimelineItem
			err := gob.NewDecoder(r).Decode(&ti)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not gob decode timeline item: %w", err))
				return
			}

			tt <- ti
		}(bytes.NewReader(data))
	})
	if err != nil {
		return nil, fmt.Errorf("could not subscribe to timeline: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := unsub(); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not unsubcribe from timeline: %w", err))
			// don't return
		}

		close(tt)
	}()

	return tt, nil
}

func (s *Service) DeleteTimelineItem(ctx context.Context, timelineItemID string) error {
	if !types.ValidUUIDv4(timelineItemID) {
		return errs.InvalidArgumentError("invalid timeline item ID")
	}

	if err := s.authorize(ctx, ResourceKindTimelineItem, timelineItemID); err != nil {
		return err
	}

	return s.Cockroach.DeleteTimelineItem(ctx, timelineItemID)
}

func (s *Service) broadcastTimelineItem(ti types.TimelineItem) {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(ti)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not gob encode timeline item: %w", err))
		return
	}

	err = s.PubSub.Pub(timelineTopic(ti.UserID), b.Bytes())
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not publish timeline item: %w", err))
		return
	}
}

func timelineTopic(userID string) string { return "timeline_item_" + userID }
