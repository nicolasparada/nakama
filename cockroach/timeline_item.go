package cockroach

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/go-errs"
)

func (c *Cockroach) createTimelineItem(ctx context.Context, userID, postID string) (string, error) {
	const query = `
		INSERT INTO timeline (user_id, post_id)
		VALUES (@user_id, @post_id)
		RETURNING id
	`
	args := pgx.StrictNamedArgs{
		"user_id": userID,
		"post_id": postID,
	}

	timelineItemID, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("sql insert timeline item: %w", err)
	}

	return timelineItemID, nil
}

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
		sqlJoinPostReactions(args, in.UserID())}
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

	return out, applyPageInfo(&out, pageArgs, func(ti types.TimelineItem) Cursor[time.Time] {
		return Cursor[time.Time]{ID: ti.Post.ID, Value: ti.Post.CreatedAt}
	})
}

func (c *Cockroach) FanoutTimeline(ctx context.Context, postID, followeeID string) ([]types.TimelineItem, error) {
	const query = `
		INSERT INTO timeline (user_id, post_id)
		SELECT follower_id, @post_id
		FROM follows
		WHERE followee_id = @followee_id
		RETURNING id AS timeline_item_id, post_id, user_id
	`
	args := pgx.StrictNamedArgs{
		"post_id":     postID,
		"followee_id": followeeID,
	}

	timeline, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.TimelineItem])
	if err != nil {
		return nil, fmt.Errorf("sql fanout timeline: %w", err)
	}

	return timeline, nil
}

func (c *Cockroach) TimelineItemUserID(ctx context.Context, timelineItemID string) (string, error) {
	const query = `SELECT user_id FROM timeline WHERE id = @timeline_item_id`
	args := pgx.StrictNamedArgs{"timeline_item_id": timelineItemID}

	userID, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if db.IsNotFoundError(err) {
		return "", errs.NotFoundError("timeline item not found")
	}

	if err != nil {
		return "", fmt.Errorf("sql select timeline item user id: %w", err)
	}

	return userID, nil
}

func (c *Cockroach) DeleteTimelineItem(ctx context.Context, timelineItemID string) error {
	const query = `DELETE FROM timeline WHERE id = @timeline_item_id`
	args := pgx.StrictNamedArgs{"timeline_item_id": timelineItemID}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete timeline item: %w", err)
	}
	return nil
}
