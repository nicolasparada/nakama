package cockroach

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

var postColumns = [...]string{
	"posts.id",
	"posts.user_id",
	"posts.content",
	"posts.is_r18",
	"posts.attachments",
	"posts.comments_count",
	"posts.reactions",
	"posts.created_at",
	"posts.updated_at",
}

var postColumnsStr = strings.Join(postColumns[:], ", ")

func (c *Cockroach) enhancePostsWithUserReactions(ctx context.Context, posts []types.Post, userID *string) error {
	if userID == nil || len(posts) == 0 {
		return nil
	}

	postIDs := make([]string, len(posts))
	for i, post := range posts {
		postIDs[i] = post.ID
	}

	userReactions, err := c.getUserReactionsForPosts(ctx, *userID, postIDs)
	if err != nil {
		return err
	}

	for i := range posts {
		posts[i].Reactions = c.addReactedFieldToReactions(posts[i].Reactions, userReactions[posts[i].ID])
	}

	return nil
}

func (c *Cockroach) enhancePostWithUserReactions(ctx context.Context, post *types.Post, userID *string) error {
	if userID == nil {
		return nil
	}

	userReactions, err := c.getUserReactionsForPosts(ctx, *userID, []string{post.ID})
	if err != nil {
		return err
	}

	post.Reactions = c.addReactedFieldToReactions(post.Reactions, userReactions[post.ID])
	return nil
}

func (c *Cockroach) getUserReactionsForPosts(ctx context.Context, userID string, postIDs []string) (map[string]map[string]bool, error) {
	if len(postIDs) == 0 {
		return make(map[string]map[string]bool), nil
	}

	query := `
		SELECT post_id, emoji 
		FROM reactions 
		WHERE user_id = @user_id AND post_id = ANY(@post_ids)
	`

	rows, err := c.db.Query(ctx, query, pgx.NamedArgs{
		"user_id":  userID,
		"post_ids": postIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("sql select user reactions: %w", err)
	}

	type reactionRow struct {
		PostID string `db:"post_id"`
		Emoji  string `db:"emoji"`
	}

	reactions, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[reactionRow])
	if err != nil {
		return nil, fmt.Errorf("sql collect user reactions: %w", err)
	}

	userReactions := make(map[string]map[string]bool)
	for _, reaction := range reactions {
		if userReactions[reaction.PostID] == nil {
			userReactions[reaction.PostID] = make(map[string]bool)
		}
		userReactions[reaction.PostID][reaction.Emoji] = true
	}

	return userReactions, nil
}

func (c *Cockroach) addReactedFieldToReactions(reactions types.ReactionsSummary, userReactions map[string]bool) types.ReactionsSummary {
	if userReactions == nil {
		userReactions = make(map[string]bool)
	}

	for i := range reactions {
		reactions[i].Reacted = userReactions[reactions[i].Emoji]
	}

	return reactions
}

func (c *Cockroach) CreatePost(ctx context.Context, in types.CreatePost) (types.Created, error) {
	var out types.Created
	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = c.createPost(ctx, in)
		if err != nil {
			return err
		}

		return c.createFeedItem(ctx, types.FanoutPost{
			UserID: in.UserID(),
			PostID: out.ID,
		})
	})
}

func (c *Cockroach) createPost(ctx context.Context, in types.CreatePost) (types.Created, error) {
	var out types.Created

	const q = `
		INSERT INTO posts (id, user_id, content, is_r18, attachments)
		VALUES (@post_id, @user_id, @content, @is_r18, @attachments)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"post_id":     id.Generate(),
		"user_id":     in.UserID(),
		"content":     in.Content,
		"is_r18":      in.IsR18,
		"attachments": in.ProcessedAttachments(),
	})
	if err != nil {
		return out, fmt.Errorf("sql insert post: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted post: %w", err)
	}

	return out, nil
}

func (c *Cockroach) createFeedItem(ctx context.Context, in types.FanoutPost) error {
	const q = `
		INSERT INTO feed (user_id, post_id)
		VALUES (@user_id, @post_id)
	`

	_, err := c.db.Exec(ctx, q, pgx.NamedArgs{
		"user_id": in.UserID,
		"post_id": in.PostID,
	})
	if err != nil {
		return fmt.Errorf("sql insert feed: %w", err)
	}

	return nil
}

func (c *Cockroach) Feed(ctx context.Context, in types.ListFeed) (types.Page[types.Post], error) {
	var out types.Page[types.Post]

	query := `
		SELECT
			` + postColumnsStr + `,
			to_json(users) AS user
		FROM feed
		INNER JOIN posts ON feed.post_id = posts.id
		INNER JOIN users ON posts.user_id = users.id
		WHERE feed.user_id = @user_id
		ORDER BY posts.id DESC
	`

	rows, err := c.db.Query(ctx, query, pgx.StrictNamedArgs{
		"user_id": in.UserID(),
	})
	if err != nil {
		return out, fmt.Errorf("sql select feed: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Post])
	if err != nil {
		return out, fmt.Errorf("sql collect feed: %w", err)
	}

	userID := in.UserID()
	if err := c.enhancePostsWithUserReactions(ctx, out.Items, &userID); err != nil {
		return out, fmt.Errorf("enhance posts with user reactions: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Posts(ctx context.Context, in types.ListPosts) (types.Page[types.Post], error) {
	var out types.Page[types.Post]

	query := `
		SELECT 
			` + postColumnsStr + `,
			to_json(users) AS user
		FROM posts
		INNER JOIN users ON posts.user_id = users.id`

	args := pgx.NamedArgs{}

	if in.Username != nil {
		query += `
		WHERE users.username = @username`
		args["username"] = *in.Username
	}

	query += `
		ORDER BY posts.id DESC`

	rows, err := c.db.Query(ctx, query, args)
	if err != nil {
		return out, fmt.Errorf("sql select posts: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Post])
	if err != nil {
		return out, fmt.Errorf("sql collect posts: %w", err)
	}

	if err := c.enhancePostsWithUserReactions(ctx, out.Items, in.LoggedInUserID()); err != nil {
		return out, fmt.Errorf("enhance posts with user reactions: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Post(ctx context.Context, in types.RetrievePost) (types.Post, error) {
	var out types.Post

	query := `
		SELECT
			` + postColumnsStr + `,
			to_json(users) AS user
		FROM posts
		INNER JOIN users ON posts.user_id = users.id
		WHERE posts.id = @post_id
	`

	rows, err := c.db.Query(ctx, query, pgx.NamedArgs{
		"post_id": in.PostID,
	})
	if err != nil {
		return out, fmt.Errorf("sql select post: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Post])
	if err != nil {
		return out, fmt.Errorf("sql collect post: %w", err)
	}

	if err := c.enhancePostWithUserReactions(ctx, &out, in.LoggedInUserID()); err != nil {
		return out, fmt.Errorf("enhance post with user reactions: %w", err)
	}

	return out, nil
}

func (c *Cockroach) FanoutPost(ctx context.Context, in types.FanoutPost) error {
	const q = `
		INSERT INTO feed (user_id, post_id)
		SELECT follows.follower_id, @post_id
		FROM follows
		WHERE follows.followee_id = @user_id
	`

	_, err := c.db.Exec(ctx, q, pgx.NamedArgs{
		"user_id": in.UserID,
		"post_id": in.PostID,
	})
	if err != nil {
		return fmt.Errorf("sql fanout feed: %w", err)
	}

	return nil
}

func (c *Cockroach) SearchPosts(ctx context.Context, in types.SearchPosts) (types.Page[types.Post], error) {
	var out types.Page[types.Post]

	query := `
		SELECT 
			` + postColumnsStr + `,
			to_json(users) AS user
		FROM posts
		INNER JOIN users ON posts.user_id = users.id
		WHERE (
			posts.content ILIKE '%' || @query || '%'
			OR
			similarity(posts.content, @query) > 0.1
		)
		ORDER BY 
			CASE WHEN posts.content ILIKE '%' || @query || '%' THEN 1 ELSE 2 END,
			similarity(posts.content, @query) DESC,
			posts.created_at DESC
	`

	rows, err := c.db.Query(ctx, query, pgx.StrictNamedArgs{
		"query": in.Query,
	})
	if err != nil {
		return out, fmt.Errorf("sql search posts: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Post])
	if err != nil {
		return out, fmt.Errorf("sql collect searched posts: %w", err)
	}

	if err := c.enhancePostsWithUserReactions(ctx, out.Items, in.LoggedInUserID()); err != nil {
		return out, fmt.Errorf("enhance posts with user reactions: %w", err)
	}

	return out, nil
}

func (c *Cockroach) ToggleReaction(ctx context.Context, in types.ToggleReaction) (inserted bool, err error) {
	return inserted, c.db.RunTx(ctx, func(ctx context.Context) error {
		exists, err := c.reactionExists(ctx, in)
		if err != nil {
			return err
		}

		if exists {
			return c.deleteReaction(ctx, in)
		}

		inserted = true
		return c.insertReaction(ctx, in)
	})
}

func (c *Cockroach) reactionExists(ctx context.Context, in types.ToggleReaction) (bool, error) {
	var exists bool
	err := c.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM reactions 
			WHERE user_id = @user_id AND post_id = @post_id AND emoji = @emoji
		)
	`, pgx.StrictNamedArgs{
		"user_id": in.LoggedInUserID(),
		"post_id": in.PostID,
		"emoji":   in.Emoji,
	}).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sql check reaction exists: %w", err)
	}
	return exists, nil
}

func (c *Cockroach) insertReaction(ctx context.Context, in types.ToggleReaction) error {
	_, err := c.db.Exec(ctx, `
		INSERT INTO reactions (user_id, post_id, emoji) 
		VALUES (@user_id, @post_id, @emoji)
	`, pgx.StrictNamedArgs{
		"user_id": in.LoggedInUserID(),
		"post_id": in.PostID,
		"emoji":   in.Emoji,
	})
	if err != nil {
		return fmt.Errorf("sql insert reaction: %w", err)
	}
	return nil
}

func (c *Cockroach) deleteReaction(ctx context.Context, in types.ToggleReaction) error {
	_, err := c.db.Exec(ctx, `
		DELETE FROM reactions 
		WHERE user_id = @user_id AND post_id = @post_id AND emoji = @emoji
	`, pgx.StrictNamedArgs{
		"user_id": in.LoggedInUserID(),
		"post_id": in.PostID,
		"emoji":   in.Emoji,
	})
	if err != nil {
		return fmt.Errorf("sql delete reaction: %w", err)
	}
	return nil
}
