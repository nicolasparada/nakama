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

const commentsColumns = `
	  comments.id
	, comments.user_id
	, comments.post_id
	, comments.content
	, comments.created_at
`

func (c *Cockroach) CreateComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		created, err := c.createComment(ctx, in)
		if err != nil {
			return err
		}

		if err := c.UpsertPostSubscription(ctx, in.UserID(), in.PostID); err != nil {
			return err
		}

		if err := c.CreatePostTags(ctx, types.CreatePostTags{
			PostID:    in.PostID,
			CommentID: &created.ID,
			Tags:      in.Tags(),
		}); err != nil {
			return err
		}

		if err := c.IncreasePostCommentsCount(ctx, in.PostID); err != nil {
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

	args := pgx.StrictNamedArgs{
		"post_id":      in.PostID,
		"auth_user_id": in.AuthUserID(),
	}

	var userReactionsJoin string
	if in.AuthUserID() != nil {
		// This join produces something like this:
		// {
		//   "emoji:❤️": true
		// }
		userReactionsJoin = `
			LEFT JOIN (
				SELECT
					comment_reactions.comment_id,
					jsonb_object_agg(comment_reactions.type || ':' || comment_reactions.reaction, true) AS reactions
				FROM comment_reactions
				WHERE comment_reactions.user_id = @auth_user_id
				GROUP BY comment_reactions.comment_id
			) AS user_reactions ON user_reactions.comment_id = comments.id
		`
	}

	pageArgs, err := ParsePageArgs[time.Time](in.PageArgs)
	if err != nil {
		return out, err
	}

	filters := []string{"comments.post_id = @post_id"}

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

	query := `
		SELECT ` + commentsColumns + `, ` + userJSONB + `
			, (@auth_user_id IS NOT NULL AND comments.user_id = @auth_user_id) AS mine
		`

	// This select produces something like this:
	// [
	//   { "type": "emoji", "reaction": "❤️", "count": 3, "reacted": true },
	//   { "type": "emoji", "reaction": "😂", "count": 2, "reacted": false }
	// ]
	query += `
		, CASE
			WHEN comments.reactions IS NULL THEN NULL
			WHEN @auth_user_id IS NULL THEN comments.reactions
			ELSE (
				SELECT jsonb_agg(
					jsonb_set(
						reaction,
						'{reacted}',
						to_jsonb(COALESCE(user_reactions.reactions, '{}'::jsonb) ? ((reaction ->> 'type') || ':' || (reaction ->> 'reaction'))),
						true
					)
				)
				FROM jsonb_array_elements(comments.reactions) AS reaction
			)
			END AS reactions
		`
	query += `
		FROM comments
		INNER JOIN users ON comments.user_id = users.id
		` + userReactionsJoin + `
		WHERE ` + strings.Join(filters, " AND ") + `
		` + order + `
		` + limit

	comments, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Comment])
	if err != nil {
		return out, fmt.Errorf("sql select comments: %w", err)
	}

	out.Items = comments
	applyPageInfo(&out, pageArgs, func(c types.Comment) Cursor[time.Time] {
		return Cursor[time.Time]{
			ID:    c.ID,
			Value: c.CreatedAt,
		}
	})

	return out, nil
}
