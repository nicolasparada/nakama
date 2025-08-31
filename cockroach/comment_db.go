package cockroach

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

var commentColumns = [...]string{
	"comments.id",
	"comments.user_id",
	"comments.post_id",
	"comments.content",
	"comments.attachment",
	"comments.reaction_counters",
	"comments.created_at",
	"comments.updated_at",
}

var commentColumnsStr = strings.Join(commentColumns[:], ", ")

func (c *Cockroach) CreateComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		var err error
		if out, err = c.createComment(ctx, in); err != nil {
			return err
		}

		return c.upsertPostSubscription(ctx, types.UpsertPostSubscription{
			UserID: in.UserID(),
			PostID: in.PostID,
		})
	})
}

func (c *Cockroach) createComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	const q = `
		INSERT INTO comments (id, user_id, post_id, content, attachment)
		VALUES (@comment_id, @user_id, @post_id, @content, @attachment)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"comment_id": id.Generate(),
		"user_id":    in.UserID(),
		"post_id":    in.PostID,
		"content":    in.Content,
		"attachment": in.Attachment(),
	})
	if db.IsForeignKeyViolationError(err, "post_id") {
		return out, errs.NewNotFoundError("post not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql insert comment: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted comment: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Comments(ctx context.Context, in types.ListComments) (types.Page[types.Comment], error) {
	var out types.Page[types.Comment]

	query := `
		SELECT
			` + commentColumnsStr + `,
			to_json(users) AS user
		FROM comments
		INNER JOIN users ON comments.user_id = users.id
		WHERE comments.post_id = @post_id
	`
	args := pgx.StrictNamedArgs{"post_id": in.PostID}

	query = addPageFilter(query, "comments", args, in.PageArgs)
	query = addPageOrder(query, "comments", in.PageArgs)
	query = addLimit(query, args, in.PageArgs)

	rows, err := c.db.Query(ctx, query, args)
	if err != nil {
		return out, fmt.Errorf("sql select comments: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Comment])
	if err != nil {
		return out, fmt.Errorf("sql collect comments: %w", err)
	}

	applyPageInfo(&out, in.PageArgs, func(c types.Comment) string { return c.ID })

	if err := c.enhanceCommentsWithUserReactions(ctx, out.Items, in.LoggedInUserID()); err != nil {
		return out, fmt.Errorf("enhance comments with user reactions: %w", err)
	}

	return out, nil
}

func (c *Cockroach) ToggleCommentReaction(ctx context.Context, in types.ToggleCommentReaction) (inserted bool, err error) {
	return inserted, c.db.RunTx(ctx, func(ctx context.Context) error {
		exists, err := c.commentReactionExists(ctx, in)
		if err != nil {
			return err
		}

		if exists {
			return c.deleteCommentReaction(ctx, in)
		}

		inserted = true
		return c.insertCommentReaction(ctx, in)
	})
}

func (c *Cockroach) commentReactionExists(ctx context.Context, in types.ToggleCommentReaction) (bool, error) {
	var exists bool
	err := c.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM comment_reactions 
			WHERE user_id = @user_id AND comment_id = @comment_id AND emoji = @emoji
		)
	`, pgx.StrictNamedArgs{
		"user_id":    in.LoggedInUserID(),
		"comment_id": in.CommentID,
		"emoji":      in.Emoji,
	}).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sql check comment reaction exists: %w", err)
	}
	return exists, nil
}

func (c *Cockroach) insertCommentReaction(ctx context.Context, in types.ToggleCommentReaction) error {
	_, err := c.db.Exec(ctx, `
		INSERT INTO comment_reactions (user_id, comment_id, emoji) 
		VALUES (@user_id, @comment_id, @emoji)
	`, pgx.StrictNamedArgs{
		"user_id":    in.LoggedInUserID(),
		"comment_id": in.CommentID,
		"emoji":      in.Emoji,
	})
	if err != nil {
		return fmt.Errorf("sql insert comment reaction: %w", err)
	}
	return nil
}

func (c *Cockroach) deleteCommentReaction(ctx context.Context, in types.ToggleCommentReaction) error {
	_, err := c.db.Exec(ctx, `
		DELETE FROM comment_reactions 
		WHERE user_id = @user_id AND comment_id = @comment_id AND emoji = @emoji
	`, pgx.StrictNamedArgs{
		"user_id":    in.LoggedInUserID(),
		"comment_id": in.CommentID,
		"emoji":      in.Emoji,
	})
	if err != nil {
		return fmt.Errorf("sql delete comment reaction: %w", err)
	}
	return nil
}

func (c *Cockroach) enhanceCommentsWithUserReactions(ctx context.Context, comments []types.Comment, userID *string) error {
	if userID == nil || len(comments) == 0 {
		return nil
	}

	commentIDs := make([]string, len(comments))
	for i, comment := range comments {
		commentIDs[i] = comment.ID
	}

	userReactions, err := c.getUserReactionsForComments(ctx, *userID, commentIDs)
	if err != nil {
		return err
	}

	for i := range comments {
		comments[i].ReactionCounters = c.addReactedFieldToCommentReactions(comments[i].ReactionCounters, userReactions[comments[i].ID])
	}

	return nil
}

func (c *Cockroach) getUserReactionsForComments(ctx context.Context, userID string, commentIDs []string) (map[string]map[string]bool, error) {
	if len(commentIDs) == 0 {
		return make(map[string]map[string]bool), nil
	}

	query := `
		SELECT comment_id, emoji 
		FROM comment_reactions 
		WHERE user_id = @user_id AND comment_id = ANY(@comment_ids)
	`

	rows, err := c.db.Query(ctx, query, pgx.NamedArgs{
		"user_id":     userID,
		"comment_ids": commentIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("sql select user comment reactions: %w", err)
	}

	type commentReactionRow struct {
		CommentID string `db:"comment_id"`
		Emoji     string `db:"emoji"`
	}

	reactions, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[commentReactionRow])
	if err != nil {
		return nil, fmt.Errorf("sql collect user comment reactions: %w", err)
	}

	userReactions := make(map[string]map[string]bool)
	for _, reaction := range reactions {
		if userReactions[reaction.CommentID] == nil {
			userReactions[reaction.CommentID] = make(map[string]bool)
		}
		userReactions[reaction.CommentID][reaction.Emoji] = true
	}

	return userReactions, nil
}

func (c *Cockroach) addReactedFieldToCommentReactions(reactions types.ReactionCounters, userReactions map[string]bool) types.ReactionCounters {
	if userReactions == nil {
		userReactions = make(map[string]bool)
	}

	for i := range reactions {
		reactions[i].Reacted = userReactions[reactions[i].Emoji]
	}

	return reactions
}
