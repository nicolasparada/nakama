package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/nakamauwu/nakama/cockroach"
	"github.com/nakamauwu/nakama/cursor"
	"github.com/nakamauwu/nakama/textutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

const commentContentMaxLength = 2048

var (
	ErrInvalidCommentID    = errs.InvalidArgumentError("invalid comment ID")
	ErrCommentNotFound     = errs.NotFoundError("comment not found")
	ErrUpdateCommentDenied = errs.PermissionDeniedError("update comment denied")
)

// CreateComment on a post.
func (s *Service) CreateComment(ctx context.Context, in types.CreateComment) (types.Comment, error) {
	var c types.Comment
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return c, errs.Unauthenticated
	}

	if err := in.Validate(); err != nil {
		return c, err
	}

	in.SetUserID(uid)
	in.SetTags(textutil.CollectTags(in.Content))

	created, err := s.Cockroach.CreateComment(ctx, in)
	if err != nil {
		return c, err
	}

	c.ID = created.ID
	c.CreatedAt = created.CreatedAt

	c.UserID = uid
	c.PostID = in.PostID
	c.Content = in.Content
	c.Mine = true

	go s.commentCreated(c)

	return c, nil
}

func (s *Service) commentCreated(c types.Comment) {
	u, err := s.userByID(context.Background(), c.UserID)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not fetch comment user: %w", err))
		return
	}

	c.User = &u
	c.Mine = false

	go s.notifyComment(c)
	go s.notifyCommentMention(c)
	go s.broadcastComment(c)
}

// Comments from a post in descending order with backward pagination.
func (s *Service) Comments(ctx context.Context, postID string, last uint64, before *string) (types.Comments, error) {
	if !types.ValidUUIDv4(postID) {
		return nil, ErrInvalidPostID
	}

	var beforeCommentID string
	var beforeCreatedAt time.Time

	if before != nil {
		var err error
		beforeCommentID, beforeCreatedAt, err = cursor.Decode(*before)
		if err != nil || !types.ValidUUIDv4(beforeCommentID) {
			return nil, ErrInvalidCursor
		}
	}

	uid, auth := ctx.Value(KeyAuthUserID).(string)
	last = normalizePageSize(last)
	query, args, err := buildQuery(`
		SELECT comments.id
		, comments.content
		, comments.reactions
		, comments.created_at
		, users.username
		, users.avatar
		{{if .auth}}
		, comments.user_id = @uid AS comment_mine
		, reactions.user_reactions
		{{end}}
		FROM comments
		INNER JOIN users ON comments.user_id = users.id
		{{if .auth}}
		LEFT JOIN (
			SELECT user_id
			, comment_id
			, json_agg(json_build_object('reaction', reaction, 'type', type)) AS user_reactions
			FROM comment_reactions
			GROUP BY user_id, comment_id
		) AS reactions ON reactions.user_id = @uid AND reactions.comment_id = comments.id
		{{end}}
		WHERE comments.post_id = @postID
		{{ if and .beforeCommentID .beforeCreatedAt }}
			AND comments.created_at <= @beforeCreatedAt
			AND (
				comments.id < @beforeCommentID
					OR comments.created_at < @beforeCreatedAt
			)
		{{ end }}
		ORDER BY comments.created_at DESC, comments.id ASC
		LIMIT @last`, map[string]any{
		"auth":            auth,
		"uid":             uid,
		"postID":          postID,
		"last":            last,
		"beforeCommentID": beforeCommentID,
		"beforeCreatedAt": beforeCreatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build comments sql query: %w", err)
	}

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select comments: %w", err)
	}

	defer rows.Close()

	var cc types.Comments
	for rows.Next() {
		var c types.Comment
		var rawReactions []byte
		var rawUserReactions []byte
		var u types.User
		var avatar sql.NullString
		dest := []any{&c.ID, &c.Content, &rawReactions, &c.CreatedAt, &u.Username, &avatar}
		if auth {
			dest = append(dest, &c.Mine, &rawUserReactions)
		}
		if err = rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("could not scan comment: %w", err)
		}

		if rawReactions != nil {
			err = json.Unmarshal(rawReactions, &c.Reactions)
			if err != nil {
				return nil, fmt.Errorf("could not json unmarshall comment reactions: %w", err)
			}
		}

		if rawUserReactions != nil {
			var userReactions []userReaction
			err = json.Unmarshal(rawUserReactions, &userReactions)
			if err != nil {
				return nil, fmt.Errorf("could not json unmarshall user comment reactions: %w", err)
			}

			for i, r := range c.Reactions {
				var reacted bool
				for _, ur := range userReactions {
					if r.Type == ur.Type && r.Reaction == ur.Reaction {
						reacted = true
						break
					}
				}
				c.Reactions[i].Reacted = &reacted
			}
		}

		u.AvatarURL = s.avatarURL(avatar)
		c.User = &u
		cc = append(cc, c)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not iterate comment rows: %w", err)
	}

	return cc, nil
}

// CommentStream to receive comments in realtime.
func (s *Service) CommentStream(ctx context.Context, postID string) (<-chan types.Comment, error) {
	if !types.ValidUUIDv4(postID) {
		return nil, ErrInvalidPostID
	}

	cc := make(chan types.Comment)
	uid, auth := ctx.Value(KeyAuthUserID).(string)
	unsub, err := s.PubSub.Sub(commentTopic(postID), func(data []byte) {
		go func(r io.Reader) {
			var c types.Comment
			err := gob.NewDecoder(r).Decode(&c)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not gob decode comment: %w", err))
				return
			}

			if auth && uid == c.UserID {
				return
			}

			cc <- c
		}(bytes.NewReader(data))
	})
	if err != nil {
		return nil, fmt.Errorf("could not subscribe to comments: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := unsub(); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not unsubcribe from comments: %w", err))
			// don't return
		}
		close(cc)
	}()

	return cc, nil
}

func (s *Service) UpdateComment(ctx context.Context, in types.UpdateComment) (types.UpdatedComment, error) {
	var out types.UpdatedComment

	in.ID = strings.TrimSpace(in.ID)
	if !types.ValidUUIDv4(in.ID) {
		return out, ErrInvalidCommentID
	}

	if in.Content != nil {
		*in.Content = textutil.SmartTrim(*in.Content)
		if *in.Content == "" || utf8.RuneCountInString(*in.Content) > commentContentMaxLength {
			return out, ErrInvalidContent
		}
	}

	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return out, errs.Unauthenticated
	}

	return out, cockroach.ExecuteTx(ctx, s.DB, func(tx pgx.Tx) error {
		var isOwner bool
		query := "SELECT user_id = $1 FROM comments WHERE id = $2"
		row := tx.QueryRow(ctx, query, uid, in.ID)
		err := row.Scan(&isOwner)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCommentNotFound
		}

		if err != nil {
			return fmt.Errorf("could not sql query select comment is owner: %w", err)
		}

		if !isOwner {
			return errs.PermissionDenied
		}

		var createdAt time.Time
		query = "SELECT created_at FROM comments WHERE id = $1"
		row = tx.QueryRow(ctx, query, in.ID)
		err = row.Scan(&createdAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCommentNotFound
		}

		if err != nil {
			return fmt.Errorf("could not sql query select comment created at: %w", err)
		}

		isRecent := time.Since(createdAt) <= time.Minute*15
		if !isRecent {
			return ErrUpdateCommentDenied
		}

		query = "UPDATE comments SET content = COALESCE($1::varchar, content) WHERE id = $2 RETURNING content"
		row = tx.QueryRow(ctx, query, in.Content, in.ID)
		err = row.Scan(&out.Content)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCommentNotFound
		}

		if err != nil {
			return fmt.Errorf("could not sql update comment: %w", err)
		}

		return nil
	})
}

func (s *Service) DeleteComment(ctx context.Context, commentID string) error {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return errs.Unauthenticated
	}

	if !types.ValidUUIDv4(commentID) {
		return ErrInvalidCommentID
	}

	err := cockroach.ExecuteTx(ctx, s.DB, func(tx pgx.Tx) error {
		var postID string
		query := "SELECT post_id FROM comments WHERE id = $1 AND user_id = $2"
		row := tx.QueryRow(ctx, query, commentID, uid)
		err := row.Scan(&postID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCommentNotFound
		}

		if err != nil {
			return fmt.Errorf("could not sql query select comment to delete post id: %w", err)
		}

		query = "DELETE FROM comments WHERE id = $1"
		_, err = tx.Exec(ctx, query, commentID)
		if err != nil {
			return fmt.Errorf("could not delete comment: %w", err)
		}

		query = "UPDATE posts SET comments_count = comments_count - 1 WHERE id = $1"
		_, err = tx.Exec(ctx, query, postID)
		if err != nil {
			return fmt.Errorf("could not update post comments count after comment deletion: %w", err)
		}

		return nil
	})
	if err != nil && err != ErrCommentNotFound {
		return err
	}

	return nil
}

func (s *Service) ToggleCommentReaction(ctx context.Context, commentID string, in types.ReactionInput) ([]types.Reaction, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, errs.Unauthenticated
	}

	if !types.ValidUUIDv4(commentID) {
		return nil, ErrInvalidCommentID
	}

	if in.Type != "emoji" || in.Reaction == "" {
		return nil, ErrInvalidReaction
	}

	if in.Type == "emoji" && !validEmoji(in.Reaction) {
		return nil, ErrInvalidReaction
	}

	var out []types.Reaction
	err := cockroach.ExecuteTx(ctx, s.DB, func(tx pgx.Tx) error {
		out = nil

		var rawReactions []byte
		var rawUserReactions []byte
		query := `
			SELECT comments.reactions, reactions.user_reactions
			FROM comments
			LEFT JOIN (
				SELECT user_id
				, comment_id
				, json_agg(json_build_object('reaction', reaction, 'type', type)) AS user_reactions
				FROM comment_reactions
				GROUP BY user_id, comment_id
			) AS reactions ON reactions.user_id = $1 AND reactions.comment_id = comments.id
			WHERE comments.id = $2`
		row := tx.QueryRow(ctx, query, uid, commentID)
		err := row.Scan(&rawReactions, &rawUserReactions)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCommentNotFound
		}

		if err != nil {
			return fmt.Errorf("could not sql scan comment and user reactions: %w", err)
		}

		var reactions []types.Reaction
		if rawReactions != nil {
			err = json.Unmarshal(rawReactions, &reactions)
			if err != nil {
				return fmt.Errorf("could not json unmarshall comment reactions: %w", err)
			}
		}

		var userReactions []userReaction
		if rawUserReactions != nil {
			err = json.Unmarshal(rawUserReactions, &userReactions)
			if err != nil {
				return fmt.Errorf("could not json unmarshall user comment reactions: %w", err)
			}
		}

		userReactionIdx := -1
		for i, ur := range userReactions {
			if ur.Type == in.Type && ur.Reaction == in.Reaction {
				userReactionIdx = i
				break
			}
		}

		reacted := userReactionIdx != -1
		if !reacted {
			query = "INSERT INTO comment_reactions (user_id, comment_id, type, reaction) VALUES ($1, $2, $3, $4)"
			_, err = tx.Exec(ctx, query, uid, commentID, in.Type, in.Reaction)
			if err != nil {
				return fmt.Errorf("could not sql insert comment reaction: %w", err)
			}
		} else {
			query = `
				DELETE FROM comment_reactions
				WHERE user_id = $1
					AND comment_id = $2
					AND type = $3
					AND reaction = $4
			`
			_, err = tx.Exec(ctx, query, uid, commentID, in.Type, in.Reaction)
			if err != nil {
				return fmt.Errorf("could not sql delete comment reaction: %w", err)
			}
		}

		if reacted {
			userReactions = append(userReactions[:userReactionIdx], userReactions[userReactionIdx+1:]...)
		} else {
			userReactions = append(userReactions, userReaction{
				Type:     in.Type,
				Reaction: in.Reaction,
			})
		}

		var updated bool
		zeroReactionsIdx := -1
		for i, r := range reactions {
			if !(r.Type == in.Type && r.Reaction == in.Reaction) {
				continue
			}

			if !reacted {
				reactions[i].Count++
			} else {
				reactions[i].Count--
				if reactions[i].Count == 0 {
					zeroReactionsIdx = i
				}
			}
			updated = true
			break
		}

		if !updated {
			reactions = append(reactions, types.Reaction{
				Type:     in.Type,
				Reaction: in.Reaction,
				Count:    1,
			})
		}

		if zeroReactionsIdx != -1 {
			reactions = append(reactions[:zeroReactionsIdx], reactions[zeroReactionsIdx+1:]...)
		}

		rawReactions, err = json.Marshal(reactions)
		if err != nil {
			return fmt.Errorf("could not json marshall comment reactions: %w", err)
		}

		query = "UPDATE comments SET reactions = $1 WHERE comments.id = $2"
		_, err = tx.Exec(ctx, query, rawReactions, commentID)
		if err != nil {
			return fmt.Errorf("could not sql update comment reactions: %w", err)
		}

		if len(userReactions) != 0 {
			for i, r := range reactions {
				var reacted bool
				for _, ur := range userReactions {
					if r.Type == ur.Type && r.Reaction == ur.Reaction {
						reacted = true
						break
					}
				}
				reactions[i].Reacted = &reacted
			}
		}

		out = reactions

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (s *Service) broadcastComment(c types.Comment) {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(c)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not gob encode comment: %w", err))
		return
	}

	err = s.PubSub.Pub(commentTopic(c.PostID), b.Bytes())
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not publish comment: %w", err))
		return
	}
}

func commentTopic(postID string) string { return "comment_" + postID }
