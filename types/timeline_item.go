package types

type TimelineItem struct {
	ID     string `json:"timelineItemID" db:"timeline_item_id"`
	UserID string `json:"-" db:"-"`
	PostID string `json:"-" db:"-"`
	Post
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
