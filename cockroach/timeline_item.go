package cockroach

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
)

func (c *Cockroach) Timeline(ctx context.Context, in types.ListTimeline) (types.Page[types.TimelineItem], error) {
	var out types.Page[types.TimelineItem]

	args := pgx.StrictNamedArgs{
		"viewer_id": in.UserID(),
	}
	selects := []string{
		`timeline.id AS timeline_item_id`,
		sqlPostCols,
		sqlUserJSONB,
		`(posts.user_id = @viewer_id) AS mine`,
		`(post_subscriptions.user_id IS NOT NULL) AS subscribed`,
		sqlSelectPostsReactions}
	joins := []string{
		"INNER JOIN posts ON timeline.post_id = posts.id",
		"INNER JOIN users ON posts.user_id = users.id",
		`LEFT JOIN post_subscriptions ON post_subscriptions.post_id = posts.id AND post_subscriptions.user_id = @viewer_id`,
		sqlJoinPostReactions}
	filters := []string{"timeline.user_id = @viewer_id"}

	pageArgs, err := ParsePageArgs[time.Time](in.PageArgs)
	if err != nil {
		return out, err
	}

	if pageArgs.After != nil {
		filters = append(filters, "(posts.created_at, posts.id) < (@after_created_at, @after_id)")
		args["after_created_at"] = pageArgs.After.Value
		args["after_id"] = pageArgs.After.ID
	} else if pageArgs.Before != nil {
		filters = append(filters, "(posts.created_at, posts.id) > (@before_created_at, @before_id)")
		args["before_created_at"] = pageArgs.Before.Value
		args["before_id"] = pageArgs.Before.ID
	}

	var order, limit string
	if pageArgs.IsBackwards() {
		order = "ORDER BY posts.created_at ASC, posts.id ASC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.Last, defaultPageSize)+1) // +1 to check if there's a next page
	} else {
		order = "ORDER BY posts.created_at DESC, posts.id DESC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.First, defaultPageSize)+1) // +1 to check if there's a next page
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM timeline
		%s
		WHERE %s
		%s
		%s`,
		strings.Join(selects, ",\n\t\t"),
		strings.Join(joins, "\n\t\t"),
		strings.Join(filters, " AND "),
		order,
		limit,
	)

	posts, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.TimelineItem])
	if err != nil {
		return out, fmt.Errorf("sql select timeline: %w", err)
	}

	out.Items = posts
	applyPageInfo(&out, pageArgs, func(ti types.TimelineItem) Cursor[time.Time] {
		return Cursor[time.Time]{ID: ti.ID, Value: ti.CreatedAt}
	})

	return out, nil
}
