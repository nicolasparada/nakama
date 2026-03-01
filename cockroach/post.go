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

const sqlPostCols = `
	  posts.id
	, posts.user_id
	, posts.content
	, posts.media
	, posts.spoiler_of
	, posts.nsfw
	, posts.comments_count
	, posts.created_at
	, posts.updated_at
`

// sqlSelectPostsReactions adds a `reacted` field to each reaction, producing something like this:
//
//	[
//		{ "kind": "emoji", "reaction": "❤️", "count": 3, "reacted": true },
//		{ "kind": "emoji", "reaction": "😂", "count": 2, "reacted": false }
//	]
const sqlSelectPostsReactions = `
	CASE WHEN posts.reactions IS NULL THEN NULL
	ELSE (
		SELECT jsonb_agg(
			jsonb_set(
				reaction,
				'{reacted}',
				to_jsonb(COALESCE(user_reactions.reactions, '{}'::jsonb) ? ((reaction ->> 'kind') || ':' || (reaction ->> 'reaction'))),
				true
			)
		)
		FROM jsonb_array_elements(posts.reactions) AS reaction
	) END AS reactions`

// sqlJoinPostReactions builds an object with keys being the reaction kind and value.
// Make sure to include `@viewer_id` in the query args when using this join.
// producing something like this:
//
//	{
//		"emoji:❤️": true
//	}
const sqlJoinPostReactions = `
	LEFT JOIN (
		SELECT
			post_reactions.post_id,
			jsonb_object_agg(post_reactions.kind || ':' || post_reactions.reaction, true) AS reactions
		FROM post_reactions
		WHERE post_reactions.user_id = @viewer_id
		GROUP BY post_reactions.post_id
	) AS user_reactions ON user_reactions.post_id = posts.id`

func (c *Cockroach) Posts(ctx context.Context, in types.ListPosts) (types.Page[types.Post], error) {
	var out types.Page[types.Post]

	args := pgx.StrictNamedArgs{}
	selects := []string{sqlPostCols, sqlUserJSONB}
	joins := []string{"INNER JOIN users ON posts.user_id = users.id"}
	filters := []string{}

	if in.Username != nil {
		args["username"] = *in.Username
		filters = append(filters, "users.username = @username")
	}

	if in.Tag != nil {
		args["tag"] = *in.Tag
		joins = append(joins, "INNER JOIN post_tags ON post_tags.post_id = posts.id AND post_tags.tag = @tag")
	}

	if in.ViewerID() != nil {
		args["viewer_id"] = *in.ViewerID()
		selects = append(selects,
			`(posts.user_id = @viewer_id) AS mine`,
			`(post_subscriptions.user_id IS NOT NULL) AS subscribed`,
			sqlSelectPostsReactions)
		joins = append(joins,
			`LEFT JOIN post_subscriptions ON post_subscriptions.post_id = posts.id AND post_subscriptions.user_id = @viewer_id`,
			sqlJoinPostReactions)
	} else {
		selects = append(selects,
			`false AS mine`,
			`false AS subscribed`,
			`posts.reactions`)
	}

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

	var condWhere string
	if len(filters) > 0 {
		condWhere = " WHERE " + strings.Join(filters, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM posts
		%s
		%s
		%s
		%s`,
		strings.Join(selects, ",\n\t\t"),
		strings.Join(joins, "\n\t\t"),
		condWhere,
		order,
		limit,
	)

	posts, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Post])
	if err != nil {
		return out, fmt.Errorf("sql select posts: %w", err)
	}

	out.Items = posts
	applyPageInfo(&out, pageArgs, func(p types.Post) Cursor[time.Time] {
		return Cursor[time.Time]{ID: p.ID, Value: p.CreatedAt}
	})

	return out, nil
}

func (c *Cockroach) increasePostCommentsCount(ctx context.Context, postID string) error {
	const query = "UPDATE posts SET comments_count = comments_count + 1 WHERE id = @post_id"
	_, err := c.db.Exec(ctx, query, pgx.StrictNamedArgs{"post_id": postID})
	if err != nil {
		return fmt.Errorf("sql increase post comments count: %w", err)
	}

	return nil
}

func (c *Cockroach) decreasePostCommentsCount(ctx context.Context, postID string) error {
	const query = "UPDATE posts SET comments_count = comments_count - 1 WHERE id = @post_id"
	_, err := c.db.Exec(ctx, query, pgx.StrictNamedArgs{"post_id": postID})
	if err != nil {
		return fmt.Errorf("sql decrease post comments count: %w", err)
	}

	return nil
}
