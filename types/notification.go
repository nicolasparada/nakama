package types

import (
	"time"

	"github.com/nakamauwu/nakama/cursor"
)

type Notification struct {
	ID       string    `json:"id"`
	UserID   string    `json:"-"`
	Actors   []string  `json:"actors"`
	Type     string    `json:"type"`
	PostID   *string   `json:"postID,omitempty" db:"post_id,omitempty"`
	Read     bool      `json:"read"`
	IssuedAt time.Time `json:"issuedAt" db:"issued_at"`
}

type Notifications []Notification

func (pp Notifications) EndCursor() *string {
	if len(pp) == 0 {
		return nil
	}

	last := pp[len(pp)-1]
	return new(cursor.Encode(last.ID, last.IssuedAt))
}
