package types

import (
	"github.com/nakamauwu/nakama/cursor"
)

type TimelineItem struct {
	ID     string `json:"timelineItemID"`
	UserID string `json:"-"`
	PostID string `json:"-"`
	*Post
}

type Timeline []TimelineItem

func (tt Timeline) EndCursor() *string {
	if len(tt) == 0 {
		return nil
	}

	last := tt[len(tt)-1]
	if last.Post == nil {
		return nil
	}

	return new(cursor.Encode(last.Post.ID, last.Post.CreatedAt))
}
