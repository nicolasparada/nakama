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

func (c *Cockroach) Post(ctx context.Context, in types.RetrievePost) (types.Post, error) {
	var out types.Post

	args := pgx.StrictNamedArgs{"post_id": in.PostID}
	selects := []string{sqlPostCols, sqlUserJSONB}
	joins := []string{"INNER JOIN users ON posts.user_id = users.id"}
	filters := []string{"posts.id = @post_id"}

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

	query := fmt.Sprintf(`
		SELECT %s
		FROM posts
		%s
		WHERE %s`,
		strings.Join(selects, ",\n\t\t"),
		strings.Join(joins, "\n\t\t"),
		strings.Join(filters, " AND "),
	)

	post, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Post])
	if db.IsNotFoundError(err) {
		return out, errs.NotFoundError("post not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql select post: %w", err)
	}

	return post, nil
}

func (c *Cockroach) PostUserID(ctx context.Context, postID string) (string, error) {
	const query = "SELECT user_id FROM posts WHERE id = @post_id"
	args := pgx.StrictNamedArgs{"post_id": postID}
	userID, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if db.IsNotFoundError(err) {
		return "", errs.NotFoundError("post not found")
	}

	if err != nil {
		return "", fmt.Errorf("sql select post user ID: %w", err)
	}

	return userID, nil
}

func (c *Cockroach) PostCreatedAt(ctx context.Context, postID string) (time.Time, error) {
	const query = "SELECT created_at FROM posts WHERE id = @post_id"
	args := pgx.StrictNamedArgs{"post_id": postID}
	createdAt, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[time.Time])
	if db.IsNotFoundError(err) {
		return createdAt, errs.NotFoundError("post not found")
	}

	if err != nil {
		return createdAt, fmt.Errorf("sql select post created at: %w", err)
	}

	return createdAt, nil
}

func (c *Cockroach) UpdatePost(ctx context.Context, in types.UpdatePost) (types.UpdatedPost, error) {
	var out types.UpdatedPost

	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = c.updatePost(ctx, in)
		if err != nil {
			return err
		}

		if in.Content != nil {
			if err := c.deletePostsTags(ctx, in.ID); err != nil {
				return err
			}
		}

		if len(in.Tags()) > 0 {
			err = c.createPostTags(ctx, types.CreatePostTags{
				PostID: in.ID,
				Tags:   in.Tags(),
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (c *Cockroach) updatePost(ctx context.Context, in types.UpdatePost) (types.UpdatedPost, error) {
	var out types.UpdatedPost

	const query = `
		UPDATE posts
		SET
			  content = COALESCE(@content, content)
			, spoiler_of = COALESCE(@spoiler_of, spoiler_of)
			, nsfw = COALESCE(@nsfw, nsfw)
			, updated_at = now()
		WHERE id = @post_id
		RETURNING content, spoiler_of, nsfw, updated_at
	`
	args := pgx.StrictNamedArgs{
		"post_id":    in.ID,
		"content":    in.Content,
		"spoiler_of": in.SpoilerOf,
		"nsfw":       in.NSFW,
	}
	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.UpdatedPost])
	if db.IsNotFoundError(err) {
		return out, errs.NotFoundError("post not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql update post: %w", err)
	}

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

func (c *Cockroach) DeletePost(ctx context.Context, postID string) error {
	const query = `
		DELETE FROM posts
		WHERE id = @post_id
	`
	args := pgx.StrictNamedArgs{"post_id": postID}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete post: %w", err)
	}

	return nil
}

func (c *Cockroach) TogglePostReaction(ctx context.Context, in types.TogglePostReaction) ([]types.Reaction, error) {
	var out []types.Reaction

	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		exists, err := c.postReactionExists(ctx, in)
		if err != nil {
			return err
		}

		if exists {
			if err := c.deletePostReaction(ctx, in); err != nil {
				return err
			}
		} else {
			if err := c.createPostReaction(ctx, in); err != nil {
				return err
			}
		}

		if err := c.refreshPostReactions(ctx, in.PostID); err != nil {
			return err
		}

		out, err = c.postReactions(ctx, in.PostID, in.UserID())
		return err
	})
}

func (c *Cockroach) postReactionExists(ctx context.Context, in types.TogglePostReaction) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM post_reactions
			WHERE user_id = @user_id AND post_id = @post_id AND kind = @kind AND reaction = @reaction
		)
	`
	args := pgx.StrictNamedArgs{
		"user_id":  in.UserID(),
		"post_id":  in.PostID,
		"kind":     in.Kind,
		"reaction": in.Reaction,
	}
	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql select post reaction exists: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) deletePostReaction(ctx context.Context, in types.TogglePostReaction) error {
	const query = `
		DELETE FROM post_reactions
		WHERE user_id = @user_id AND post_id = @post_id AND kind = @kind AND reaction = @reaction
	`
	args := pgx.StrictNamedArgs{
		"user_id":  in.UserID(),
		"post_id":  in.PostID,
		"kind":     in.Kind,
		"reaction": in.Reaction,
	}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete post reaction: %w", err)
	}

	return nil
}

func (c *Cockroach) createPostReaction(ctx context.Context, in types.TogglePostReaction) error {
	const query = `
		INSERT INTO post_reactions (user_id, post_id, kind, reaction)
		VALUES (@user_id, @post_id, @kind, @reaction)
	`
	args := pgx.StrictNamedArgs{
		"user_id":  in.UserID(),
		"post_id":  in.PostID,
		"kind":     in.Kind,
		"reaction": in.Reaction,
	}
	_, err := c.db.Exec(ctx, query, args)
	// careful when being called from within a transaction.
	// This error will mark the whole transaction as failed.
	// Make sure to use [c.postReactionExists] before.
	if db.IsUniqueViolationError(err) {
		return nil // idempotent
	}

	if err != nil {
		return fmt.Errorf("sql insert post reaction: %w", err)
	}

	return nil
}

func (c *Cockroach) refreshPostReactions(ctx context.Context, postID string) error {
	const query = `
		WITH counts AS (
			SELECT kind, reaction, COUNT(*) AS count
			FROM post_reactions
			WHERE post_id = @post_id
			GROUP BY kind, reaction
		)
		UPDATE posts
		SET reactions = (
			SELECT jsonb_agg(jsonb_build_object('kind', kind, 'reaction', reaction, 'count', count))
			FROM counts
		)
		WHERE id = @post_id
	`
	args := pgx.StrictNamedArgs{
		"post_id": postID,
	}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql refresh post reactions: %w", err)
	}

	return nil
}

func (c *Cockroach) postReactions(ctx context.Context, postID, viewerID string) ([]types.Reaction, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM posts %s WHERE posts.id = @post_id`,
		sqlSelectPostsReactions,
		sqlJoinPostReactions,
	)
	args := pgx.StrictNamedArgs{
		"post_id":   postID,
		"viewer_id": viewerID,
	}
	reactions, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[[]types.Reaction])
	if err != nil {
		return nil, fmt.Errorf("sql select post reactions: %w", err)
	}

	return reactions, nil
}

func (c *Cockroach) ToggleSubscription(ctx context.Context, in types.ToggleSubscription) (types.ToggledSubscription, error) {
	var out types.ToggledSubscription
	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		exists, err := c.subscriptionExists(ctx, in)
		if err != nil {
			return err
		}

		if exists {
			if err := c.deleteSubscription(ctx, in); err != nil {
				return err
			}

			out.Subscribed = false

			return nil
		}

		if err := c.createSubscription(ctx, in); err != nil {
			return err
		}

		out.Subscribed = true

		return nil
	})
}

func (c *Cockroach) subscriptionExists(ctx context.Context, in types.ToggleSubscription) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM post_subscriptions
			WHERE user_id = @user_id AND post_id = @post_id
		)
	`
	args := pgx.StrictNamedArgs{
		"user_id": in.UserID(),
		"post_id": in.PostID,
	}
	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql select post subscription exists: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) deleteSubscription(ctx context.Context, in types.ToggleSubscription) error {
	const query = `
		DELETE FROM post_subscriptions
		WHERE user_id = @user_id AND post_id = @post_id
	`
	args := pgx.StrictNamedArgs{
		"user_id": in.UserID(),
		"post_id": in.PostID,
	}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete post subscription: %w", err)
	}

	return nil
}

func (c *Cockroach) createSubscription(ctx context.Context, in types.ToggleSubscription) error {
	const query = `
		INSERT INTO post_subscriptions (user_id, post_id)
		VALUES (@user_id, @post_id)
	`
	args := pgx.StrictNamedArgs{
		"user_id": in.UserID(),
		"post_id": in.PostID,
	}
	_, err := c.db.Exec(ctx, query, args)
	if db.IsForeignKeyViolationError(err, "post_id") {
		return errs.NotFoundError("post not found")
	}

	if err != nil {
		return fmt.Errorf("sql insert post subscription: %w", err)
	}

	return nil
}
