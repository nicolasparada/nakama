package nakama

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cockroachdb/cockroach-go/crdb"
)

var (
	// ErrInvalidPostID denotes an invalid post id; that is not uuid.
	ErrInvalidPostID = InvalidArgumentError("invalid post id")
	// ErrInvalidContent denotes an invalid content.
	ErrInvalidContent = InvalidArgumentError("invalid content")
	// ErrInvalidSpoiler denotes an invalid spoiler title.
	ErrInvalidSpoiler = InvalidArgumentError("invalid spoiler")
	// ErrPostNotFound denotes a not found post.
	ErrPostNotFound = NotFoundError("post not found")
	// ErrInvalidUpdatePostParams denotes invalid params to update a post, that is no params altogether.
	ErrInvalidUpdatePostParams = InvalidArgumentError("invalid update post params")
	// ErrInvalidCursor denotes an invalid cursor, that is not base64 encoded and has a key and timestamp separated by comma.
	ErrInvalidCursor = InvalidArgumentError("invalid cursor")
)

// Post model.
type Post struct {
	ID             string     `json:"id"`
	UserID         string     `json:"-"`
	Content        string     `json:"content"`
	SpoilerOf      *string    `json:"spoilerOf"`
	NSFW           bool       `json:"NSFW"`
	LikesCount     int        `json:"likesCount"`
	ReactionCounts []Reaction `json:"reactionCounts"`
	CommentsCount  int        `json:"commentsCount"`
	CreatedAt      time.Time  `json:"createdAt"`
	User           *User      `json:"user,omitempty"`
	Mine           bool       `json:"mine"`
	Liked          bool       `json:"liked"`
	Subscribed     bool       `json:"subscribed"`
}

// ToggleLikeOutput response.
type ToggleLikeOutput struct {
	Liked      bool `json:"liked"`
	LikesCount int  `json:"likesCount"`
}

// ToggleSubscriptionOutput response.
type ToggleSubscriptionOutput struct {
	Subscribed bool `json:"subscribed"`
}

// CreatePost publishes a post to the user timeline and fan-outs it to his followers.
func (s *Service) CreatePost(ctx context.Context, content string, spoilerOf *string, nsfw bool) (TimelineItem, error) {
	var ti TimelineItem
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return ti, ErrUnauthenticated
	}

	content = smartTrim(content)
	if content == "" || utf8.RuneCountInString(content) > 480 {
		return ti, ErrInvalidContent
	}

	if spoilerOf != nil {
		*spoilerOf = smartTrim(*spoilerOf)
		if *spoilerOf == "" || utf8.RuneCountInString(*spoilerOf) > 64 {
			return ti, ErrInvalidSpoiler
		}
	}

	var p Post
	err := crdb.ExecuteTx(ctx, s.DB, nil, func(tx *sql.Tx) error {
		query := `
			INSERT INTO posts (user_id, content, spoiler_of, nsfw) VALUES ($1, $2, $3, $4)
			RETURNING id, created_at`
		row := tx.QueryRowContext(ctx, query, uid, content, spoilerOf, nsfw)
		err := row.Scan(&p.ID, &p.CreatedAt)
		if isForeignKeyViolation(err) {
			return ErrUserGone
		}

		if err != nil {
			return fmt.Errorf("could not insert post: %w", err)
		}

		p.UserID = uid
		p.Content = content
		p.SpoilerOf = spoilerOf
		p.NSFW = nsfw
		p.Mine = true

		query = "INSERT INTO post_subscriptions (user_id, post_id) VALUES ($1, $2)"
		if _, err = tx.ExecContext(ctx, query, uid, p.ID); err != nil {
			return fmt.Errorf("could not insert post subscription: %w", err)
		}

		p.Subscribed = true

		query = "INSERT INTO timeline (user_id, post_id) VALUES ($1, $2) RETURNING id"
		err = tx.QueryRowContext(ctx, query, uid, p.ID).Scan(&ti.ID)
		if err != nil {
			return fmt.Errorf("could not insert timeline item: %w", err)
		}

		ti.UserID = uid
		ti.PostID = p.ID
		ti.Post = &p

		return nil
	})
	if err != nil {
		return ti, err
	}

	go s.postCreated(p)

	return ti, nil
}

func (s *Service) postCreated(p Post) {
	u, err := s.userByID(context.Background(), p.UserID)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not fetch post user: %w", err))
		return
	}

	p.User = &u
	p.Mine = false
	p.Subscribed = false

	go s.fanoutPost(p)
	go s.notifyPostMention(p)
}

type Posts []Post

func (pp Posts) EndCursor() *string {
	if len(pp) == 0 {
		return nil
	}

	last := pp[len(pp)-1]
	return strPtr(encodeCursor(last.ID, last.CreatedAt))
}

// Posts from a user in descending order and with backward pagination.
func (s *Service) Posts(ctx context.Context, username string, last uint64, before *string) (Posts, error) {
	username = strings.TrimSpace(username)
	if !reUsername.MatchString(username) {
		return nil, ErrInvalidUsername
	}

	var beforePostID string
	var beforeCreatedAt time.Time

	if before != nil {
		var err error
		beforePostID, beforeCreatedAt, err = decodeCursor(*before)
		if err != nil || !reUUID.MatchString(beforePostID) {
			return nil, ErrInvalidCursor
		}
	}

	uid, auth := ctx.Value(KeyAuthUserID).(string)
	last = normalizePageSize(last)
	query, args, err := buildQuery(`
		SELECT posts.id
		, posts.content
		, posts.spoiler_of
		, posts.nsfw
		, posts.likes_count
		, posts.reaction_counts
		, posts.comments_count
		, posts.created_at
		{{ if .auth }}
		, posts.user_id = @uid AS post_mine
		, likes.user_id IS NOT NULL AS post_liked
		, subscriptions.user_id IS NOT NULL AS post_subscribed
		{{ end }}
		FROM posts
		{{ if .auth }}
		LEFT JOIN post_likes AS likes
			ON likes.user_id = @uid AND likes.post_id = posts.id
		LEFT JOIN post_subscriptions AS subscriptions
			ON subscriptions.user_id = @uid AND subscriptions.post_id = posts.id
		{{ end }}
		WHERE posts.user_id = (SELECT id FROM users WHERE username = @username)
		{{ if and .beforePostID .beforeCreatedAt }}
			AND posts.created_at <= @beforeCreatedAt
			AND (
				posts.id < @beforePostID
					OR posts.created_at < @beforeCreatedAt
			)
		{{ end }}
		ORDER BY posts.created_at DESC, posts.id ASC
		LIMIT @last`, map[string]interface{}{
		"auth":            auth,
		"uid":             uid,
		"username":        username,
		"last":            last,
		"beforePostID":    beforePostID,
		"beforeCreatedAt": beforeCreatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build posts sql query: %w", err)
	}

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select posts: %w", err)
	}

	defer rows.Close()

	var pp Posts
	for rows.Next() {
		var p Post
		var rawReactionCounts []byte
		dest := []interface{}{
			&p.ID,
			&p.Content,
			&p.SpoilerOf,
			&p.NSFW,
			&p.LikesCount,
			&rawReactionCounts,
			&p.CommentsCount,
			&p.CreatedAt,
		}
		if auth {
			dest = append(dest, &p.Mine, &p.Liked, &p.Subscribed)
		}

		if err = rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("could not scan post: %w", err)
		}

		if rawReactionCounts != nil {
			err = json.Unmarshal(rawReactionCounts, &p.ReactionCounts)
			if err != nil {
				return nil, fmt.Errorf("could not json unmarshall post reaction counts: %w", err)
			}
		}

		pp = append(pp, p)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not iterate posts rows: %w", err)
	}

	return pp, nil
}

// Post with the given ID.
func (s *Service) Post(ctx context.Context, postID string) (Post, error) {
	var p Post
	if !reUUID.MatchString(postID) {
		return p, ErrInvalidPostID
	}

	uid, auth := ctx.Value(KeyAuthUserID).(string)
	query, args, err := buildQuery(`
		SELECT posts.id
		, posts.content
		, posts.spoiler_of
		, posts.nsfw
		, posts.likes_count
		, posts.reaction_counts
		, posts.comments_count
		, posts.created_at
		, users.username, users.avatar
		{{if .auth}}
		, posts.user_id = @uid AS mine
		, likes.user_id IS NOT NULL AS liked
		, subscriptions.user_id IS NOT NULL AS subscribed
		{{end}}
		FROM posts
		INNER JOIN users ON posts.user_id = users.id
		{{if .auth}}
		LEFT JOIN post_likes AS likes
			ON likes.user_id = @uid AND likes.post_id = posts.id
		LEFT JOIN post_subscriptions AS subscriptions
			ON subscriptions.user_id = @uid AND subscriptions.post_id = posts.id
		{{end}}
		WHERE posts.id = @post_id`, map[string]interface{}{
		"auth":    auth,
		"uid":     uid,
		"post_id": postID,
	})
	if err != nil {
		return p, fmt.Errorf("could not build post sql query: %w", err)
	}

	var u User
	var rawReactionCounts []byte
	var avatar sql.NullString
	dest := []interface{}{
		&p.ID,
		&p.Content,
		&p.SpoilerOf,
		&p.NSFW,
		&p.LikesCount,
		&rawReactionCounts,
		&p.CommentsCount,
		&p.CreatedAt,
		&u.Username,
		&avatar,
	}
	if auth {
		dest = append(dest, &p.Mine, &p.Liked, &p.Subscribed)
	}
	err = s.DB.QueryRowContext(ctx, query, args...).Scan(dest...)
	if err == sql.ErrNoRows {
		return p, ErrPostNotFound
	}

	if err != nil {
		return p, fmt.Errorf("could not sql query select post: %w", err)
	}

	if rawReactionCounts != nil {
		err = json.Unmarshal(rawReactionCounts, &p.ReactionCounts)
		if err != nil {
			return p, fmt.Errorf("could not json unmarshall post reaction counts: %w", err)
		}
	}

	u.AvatarURL = s.avatarURL(avatar)
	p.User = &u

	return p, nil
}

type UpdatePostParams struct {
	Content   *string
	SpoilerOf *string
	NSFW      *bool
}

func (params UpdatePostParams) Empty() bool {
	return params.Content == nil && params.NSFW == nil && params.SpoilerOf == nil
}

type UpdatedPostFields struct {
	Content   string
	SpoilerOf *string
	NSFW      bool
}

func (s *Service) UpdatePost(ctx context.Context, postID string, params UpdatePostParams) (UpdatedPostFields, error) {
	var updated UpdatedPostFields
	if params.Empty() {
		return updated, ErrInvalidUpdatePostParams
	}

	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return updated, ErrUnauthenticated
	}

	if !reUUID.MatchString(postID) {
		return updated, ErrInvalidPostID
	}

	if params.Content != nil {
		*params.Content = smartTrim(*params.Content)
		if *params.Content == "" || utf8.RuneCountInString(*params.Content) > 480 {
			return updated, ErrInvalidContent
		}
	}

	if params.SpoilerOf != nil {
		*params.SpoilerOf = smartTrim(*params.SpoilerOf)
		if *params.SpoilerOf == "" || utf8.RuneCountInString(*params.SpoilerOf) > 64 {
			return updated, ErrInvalidSpoiler
		}
	}

	var set []string
	if params.Content != nil {
		set = append(set, "content = @content")
	}
	if params.SpoilerOf != nil {
		set = append(set, "spoiler_of = @spoiler_of")
	}
	if params.NSFW != nil {
		set = append(set, "nsfw = @nsfw")
	}
	query, args, err := buildQuery(`
		UPDATE posts
		SET {{ .set }}
		WHERE id = @post_id
			AND user_id = @auth_user_id
		RETURNING content, spoiler_of, nsfw
		`, map[string]interface{}{
		"content":      params.Content,
		"spoiler_of":   params.SpoilerOf,
		"nsfw":         params.NSFW,
		"set":          strings.Join(set, ", "),
		"post_id":      postID,
		"auth_user_id": uid,
	})
	if err != nil {
		return updated, fmt.Errorf("could not sql update post: %w", err)
	}

	row := s.DB.QueryRowContext(ctx, query, args...)
	err = row.Scan(&updated.Content, &updated.SpoilerOf, &updated.NSFW)
	if err != nil {
		return updated, fmt.Errorf("could not sql update post content: %w", err)
	}

	return updated, nil
}

func (s *Service) DeletePost(ctx context.Context, postID string) error {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return ErrUnauthenticated
	}

	if !reUUID.MatchString(postID) {
		return ErrInvalidPostID
	}

	query := "DELETE FROM posts WHERE id = $1 AND user_id = $2"
	_, err := s.DB.ExecContext(ctx, query, postID, uid)
	if err != nil {
		return fmt.Errorf("could not sql delete post: %w", err)
	}

	return nil
}

// TogglePostLike 🖤
func (s *Service) TogglePostLike(ctx context.Context, postID string) (ToggleLikeOutput, error) {
	var out ToggleLikeOutput
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, ErrUnauthenticated
	}

	if !reUUID.MatchString(postID) {
		return out, ErrInvalidPostID
	}

	err := crdb.ExecuteTx(ctx, s.DB, nil, func(tx *sql.Tx) error {
		query := `
			SELECT EXISTS (
				SELECT 1 FROM post_likes WHERE user_id = $1 AND post_id = $2
			)`
		err := tx.QueryRowContext(ctx, query, uid, postID).Scan(&out.Liked)
		if err != nil {
			return fmt.Errorf("could not query select post like existence: %w", err)
		}

		if out.Liked {
			query = "DELETE FROM post_likes WHERE user_id = $1 AND post_id = $2"
			if _, err = tx.ExecContext(ctx, query, uid, postID); err != nil {
				return fmt.Errorf("could not delete post like: %w", err)
			}

			query = "UPDATE posts SET likes_count = likes_count - 1 WHERE id = $1 RETURNING likes_count"
			err = tx.QueryRowContext(ctx, query, postID).Scan(&out.LikesCount)
			if err != nil {
				return fmt.Errorf("could not update and decrement post likes count: %w", err)
			}
		} else {
			query = "INSERT INTO post_likes (user_id, post_id) VALUES ($1, $2)"
			_, err = tx.ExecContext(ctx, query, uid, postID)

			if isForeignKeyViolation(err) {
				return ErrPostNotFound
			}

			if err != nil {
				return fmt.Errorf("could not insert post like: %w", err)
			}

			query = "UPDATE posts SET likes_count = likes_count + 1 WHERE id = $1 RETURNING likes_count"
			err = tx.QueryRowContext(ctx, query, postID).Scan(&out.LikesCount)
			if err != nil {
				return fmt.Errorf("could not update and increment post likes count: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return out, err
	}

	out.Liked = !out.Liked

	return out, nil
}

// TogglePostSubscription so you can stop receiving notifications from a thread.
func (s *Service) TogglePostSubscription(ctx context.Context, postID string) (ToggleSubscriptionOutput, error) {
	var out ToggleSubscriptionOutput
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, ErrUnauthenticated
	}

	if !reUUID.MatchString(postID) {
		return out, ErrInvalidPostID
	}

	err := crdb.ExecuteTx(ctx, s.DB, nil, func(tx *sql.Tx) error {
		query := `SELECT EXISTS (
			SELECT 1 FROM post_subscriptions WHERE user_id = $1 AND post_id = $2
		)`
		err := tx.QueryRowContext(ctx, query, uid, postID).Scan(&out.Subscribed)
		if err != nil {
			return fmt.Errorf("could not query select post subscription existence: %w", err)
		}

		if out.Subscribed {
			query = "DELETE FROM post_subscriptions WHERE user_id = $1 AND post_id = $2"
			if _, err = tx.ExecContext(ctx, query, uid, postID); err != nil {
				return fmt.Errorf("could not delete post subscription: %w", err)
			}
		} else {
			query = "INSERT INTO post_subscriptions (user_id, post_id) VALUES ($1, $2)"
			_, err = tx.ExecContext(ctx, query, uid, postID)
			if isForeignKeyViolation(err) {
				return ErrPostNotFound
			}

			if err != nil {
				return fmt.Errorf("could not insert post subscription: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return out, err
	}

	out.Subscribed = !out.Subscribed

	return out, nil
}
