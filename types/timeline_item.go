package types

import "time"

type TimelineItem struct {
	ID     string `json:"timelineItemID" db:"timeline_item_id"`
	UserID string `json:"-" db:"-"`
	PostID string `json:"-" db:"-"`
	Post
}

type CreatedTimelineItem struct {
	TimelineItemID string    `json:"timelineItemID" db:"timeline_item_id"`
	PostID         string    `json:"postID" db:"post_id"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
}

type ListTimeline struct {
	PageArgs
	userID string
}

func (in *ListTimeline) SetUserID(userID string) {
	in.userID = userID
}

func (in ListTimeline) UserID() string {
	return in.userID
}

func (in *ListTimeline) Validate() error {
	return in.PageArgs.Validate()
}
