package cockroach

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/go-errs"
)

const sqlCommentCols = `
	  comments.id
	, comments.user_id
	, comments.post_id
	, comments.content
	, comments.created_at
`

// sqlSelectCommentsReactions adds a `reacted` field to each reaction, producing something like this:
//
//	[
//		{ "kind": "emoji", "reaction": "❤️", "count": 3, "reacted": true },
//		{ "kind": "emoji", "reaction": "😂", "count": 2, "reacted": false }
//	]
const sqlSelectCommentsReactions = `
	CASE WHEN comments.reactions IS NULL THEN NULL
	ELSE (
		SELECT jsonb_agg(
			jsonb_set(
				reaction,
				'{reacted}',
				to_jsonb(COALESCE(user_reactions.reactions, '{}'::jsonb) ? ((reaction ->> 'kind') || ':' || (reaction ->> 'reaction'))),
				true
			)
		)
		FROM jsonb_array_elements(comments.reactions) AS reaction
	) END AS reactions`

// sqlJoinCommentReactions builds an object with keys being the reaction kind and value.
// Make sure to include `@viewer_id` in the query args when using this join.
// producing something like this:
//
//	{
//		"emoji:❤️": true
//	}
const sqlJoinCommentReactions = `
	LEFT JOIN (
		SELECT
			comment_reactions.comment_id,
			jsonb_object_agg(comment_reactions.kind || ':' || comment_reactions.reaction, true) AS reactions
		FROM comment_reactions
		WHERE comment_reactions.user_id = @viewer_id
		GROUP BY comment_reactions.comment_id
	) AS user_reactions ON user_reactions.comment_id = comments.id`

func (c *Cockroach) CreateComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		created, err := c.createComment(ctx, in)
		if err != nil {
			return err
		}

		if err := c.upsertPostSubscription(ctx, in.UserID(), in.PostID); err != nil {
			return err
		}

		if err := c.createPostTags(ctx, types.CreatePostTags{
			PostID:    in.PostID,
			CommentID: &created.ID,
			Tags:      in.Tags(),
		}); err != nil {
			return err
		}

		if err := c.increasePostCommentsCount(ctx, in.PostID); err != nil {
			return err
		}

		out = created

		return nil
	})
}

func (c *Cockroach) createComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	const query = `
		INSERT INTO comments (user_id, post_id, content) VALUES (@user_id, @post_id, @content)
		RETURNING id, created_at
	`
	args := pgx.StrictNamedArgs{
		"user_id": in.UserID(),
		"post_id": in.PostID,
		"content": in.Content,
	}
	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Created])
	if db.IsForeignKeyViolationError(err) {
		return out, errs.NotFoundError("post not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql insert comment: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Comments(ctx context.Context, in types.ListComments) (types.Page[types.Comment], error) {
	var out types.Page[types.Comment]

	args := pgx.StrictNamedArgs{"post_id": in.PostID}
	selects := []string{sqlCommentCols, sqlUserJSONB}
	joins := []string{"INNER JOIN users ON comments.user_id = users.id"}
	filters := []string{"comments.post_id = @post_id"}

	if in.ViewerID() != nil {
		args["viewer_id"] = *in.ViewerID()
	}

	if in.ViewerID() != nil {
		args["viewer_id"] = *in.ViewerID()
		selects = append(selects, `(comments.user_id = @viewer_id) AS mine`, sqlSelectCommentsReactions)
		joins = append(joins, sqlJoinCommentReactions)
	} else {
		selects = append(selects, `false AS mine`, `comments.reactions`)
	}

	pageArgs, err := ParsePageArgs[time.Time](in.PageArgs)
	if err != nil {
		return out, err
	}

	if pageArgs.After != nil {
		filters = append(filters, "(comments.created_at, comments.id) < (@after_created_at, @after_id)")
		args["after_created_at"] = pageArgs.After.Value
		args["after_id"] = pageArgs.After.ID
	} else if pageArgs.Before != nil {
		filters = append(filters, "(comments.created_at, comments.id) > (@before_created_at, @before_id)")
		args["before_created_at"] = pageArgs.Before.Value
		args["before_id"] = pageArgs.Before.ID
	}

	var order, limit string
	if pageArgs.IsBackwards() {
		order = "ORDER BY comments.created_at ASC, comments.id ASC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.Last, defaultPageSize)+1) // +1 to check if there's a next page
	} else {
		order = "ORDER BY comments.created_at DESC, comments.id DESC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.First, defaultPageSize)+1) // +1 to check if there's a next page
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM comments
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

	comments, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Comment])
	if err != nil {
		return out, fmt.Errorf("sql select comments: %w", err)
	}

	out.Items = comments
	applyPageInfo(&out, pageArgs, func(c types.Comment) Cursor[time.Time] {
		return Cursor[time.Time]{ID: c.ID, Value: c.CreatedAt}
	})

	return out, nil
}

func (c *Cockroach) CommentUserID(ctx context.Context, commentID string) (string, error) {
	const query = `
		SELECT user_id
		FROM comments
		WHERE id = @comment_id
	`
	args := pgx.StrictNamedArgs{"comment_id": commentID}
	userID, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if db.IsNotFoundError(err) {
		return userID, errs.NotFoundError("comment not found")
	}

	if err != nil {
		return userID, fmt.Errorf("sql select comment user_id: %w", err)
	}

	return userID, nil
}

func (c *Cockroach) CommentCreatedAt(ctx context.Context, commentID string) (time.Time, error) {
	const query = `
		SELECT created_at
		FROM comments
		WHERE id = @comment_id
	`
	args := pgx.StrictNamedArgs{"comment_id": commentID}
	createdAt, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[time.Time])
	if db.IsNotFoundError(err) {
		return createdAt, errs.NotFoundError("comment not found")
	}

	if err != nil {
		return createdAt, fmt.Errorf("sql select comment created_at: %w", err)
	}

	return createdAt, nil
}

func (c *Cockroach) UpdateComment(ctx context.Context, in types.UpdateComment) (types.UpdatedComment, error) {
	var out types.UpdatedComment

	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = c.updateComment(ctx, in)
		if err != nil {
			return err
		}

		if err := c.deletePostsTagsWithComment(ctx, in.ID); err != nil {
			return err
		}

		if len(in.Tags()) > 0 {
			postID, err := c.commentPostID(ctx, in.ID)
			if err != nil {
				return err
			}

			err = c.createPostTags(ctx, types.CreatePostTags{
				PostID:    postID,
				CommentID: &in.ID,
				Tags:      in.Tags(),
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (c *Cockroach) updateComment(ctx context.Context, in types.UpdateComment) (types.UpdatedComment, error) {
	var out types.UpdatedComment

	const query = `
		UPDATE comments
		SET content = COALESCE(@content, content)
		WHERE id = @comment_id
		RETURNING content
	`
	args := pgx.StrictNamedArgs{
		"comment_id": in.ID,
		"content":    in.Content,
	}
	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.UpdatedComment])
	if db.IsNotFoundError(err) {
		return out, errs.NotFoundError("comment not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql update comment: %w", err)
	}

	return out, nil
}

func (c *Cockroach) DeleteComment(ctx context.Context, commentID string) error {
	err := c.db.RunTx(ctx, func(ctx context.Context) error {
		postID, err := c.commentPostID(ctx, commentID)
		if err != nil {
			return err
		}

		if err := c.deleteComment(ctx, commentID); err != nil {
			return err
		}

		return c.decreasePostCommentsCount(ctx, postID)
	})
	if errors.Is(err, errs.NotFound) {
		return nil // idempotent
	}

	return err
}

func (c *Cockroach) commentPostID(ctx context.Context, commentID string) (string, error) {
	const query = `
		SELECT post_id
		FROM comments
		WHERE id = @comment_id
	`
	args := pgx.StrictNamedArgs{"comment_id": commentID}
	postID, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if db.IsNotFoundError(err) {
		return postID, errs.NotFoundError("comment not found")
	}

	if err != nil {
		return postID, fmt.Errorf("sql select comment post_id: %w", err)
	}

	return postID, nil
}

func (c *Cockroach) deleteComment(ctx context.Context, commentID string) error {
	const query = `
		DELETE FROM comments
		WHERE id = @comment_id
	`
	args := pgx.StrictNamedArgs{"comment_id": commentID}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete comment: %w", err)
	}

	return nil
}

func (c *Cockroach) ToggleCommentReaction(ctx context.Context, in types.ToggleCommentReaction) ([]types.Reaction, error) {
	var out []types.Reaction

	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		exists, err := c.commentReactionExists(ctx, in)
		if err != nil {
			return err
		}

		if exists {
			if err := c.deleteCommentReaction(ctx, in); err != nil {
				return err
			}
		} else {
			if err := c.createCommentReaction(ctx, in); err != nil {
				return err
			}
		}

		if err := c.refreshCommentReactions(ctx, in.CommentID); err != nil {
			return err
		}

		out, err = c.commentReactions(ctx, in.CommentID, in.UserID())
		return err
	})
}

func (c *Cockroach) commentReactionExists(ctx context.Context, in types.ToggleCommentReaction) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM comment_reactions
			WHERE user_id = @user_id AND comment_id = @comment_id AND kind = @kind AND reaction = @reaction
		)
	`
	args := pgx.StrictNamedArgs{
		"user_id":    in.UserID(),
		"comment_id": in.CommentID,
		"kind":       in.Kind,
		"reaction":   in.Reaction,
	}
	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql select comment reaction exists: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) deleteCommentReaction(ctx context.Context, in types.ToggleCommentReaction) error {
	const query = `
		DELETE FROM comment_reactions
		WHERE user_id = @user_id AND comment_id = @comment_id AND kind = @kind AND reaction = @reaction
	`
	args := pgx.StrictNamedArgs{
		"user_id":    in.UserID(),
		"comment_id": in.CommentID,
		"kind":       in.Kind,
		"reaction":   in.Reaction,
	}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql delete comment reaction: %w", err)
	}

	return nil
}

func (c *Cockroach) createCommentReaction(ctx context.Context, in types.ToggleCommentReaction) error {
	const query = `
		INSERT INTO comment_reactions (user_id, comment_id, kind, reaction)
		VALUES (@user_id, @comment_id, @kind, @reaction)
	`
	args := pgx.StrictNamedArgs{
		"user_id":    in.UserID(),
		"comment_id": in.CommentID,
		"kind":       in.Kind,
		"reaction":   in.Reaction,
	}
	_, err := c.db.Exec(ctx, query, args)
	// careful when being called from within a transaction.
	// This error will mark the whole transaction as failed.
	// Make sure to use [c.commentReactionExists] before.
	if db.IsUniqueViolationError(err) {
		return nil // idempotent
	}

	if err != nil {
		return fmt.Errorf("sql insert comment reaction: %w", err)
	}

	return nil
}

func (c *Cockroach) refreshCommentReactions(ctx context.Context, commentID string) error {
	const query = `
		WITH counts AS (
			SELECT kind, reaction, COUNT(*) AS count
			FROM comment_reactions
			WHERE comment_id = @comment_id
			GROUP BY kind, reaction
		)
		UPDATE comments
		SET reactions = (
			SELECT jsonb_agg(jsonb_build_object('kind', kind, 'reaction', reaction, 'count', count))
			FROM counts
		)
		WHERE id = @comment_id
	`
	args := pgx.StrictNamedArgs{
		"comment_id": commentID,
	}
	_, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql refresh comment reactions: %w", err)
	}

	return nil
}

func (c *Cockroach) commentReactions(ctx context.Context, commentID, viewerID string) ([]types.Reaction, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM comments %s WHERE comments.id = @comment_id`,
		sqlSelectCommentsReactions,
		sqlJoinCommentReactions,
	)
	args := pgx.StrictNamedArgs{
		"comment_id": commentID,
		"viewer_id":  viewerID,
	}
	reactions, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[[]types.Reaction])
	if err != nil {
		return nil, fmt.Errorf("sql select comment reactions: %w", err)
	}

	return reactions, nil
}
