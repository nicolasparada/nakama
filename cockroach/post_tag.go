package cockroach

import (
	"context"
	"fmt"

	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
)

func (c *Cockroach) CreatePostTags(ctx context.Context, in types.CreatePostTags) error {
	if len(in.Tags) == 0 {
		return nil
	}

	rows := make([]map[string]any, len(in.Tags))
	for i, tag := range in.Tags {
		rows[i] = map[string]any{
			"post_id":    in.PostID,
			"comment_id": in.CommentID,
			"tag":        tag,
		}
	}

	_, err := pgxutil.Insert(ctx, c.db, "post_tags", rows)
	if err != nil {
		return fmt.Errorf("sql insert post tags: %w", err)
	}

	return nil
}
