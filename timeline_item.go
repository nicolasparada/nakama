package nakama

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"time"
	"unicode/utf8"

	"github.com/cockroachdb/cockroach-go/crdb"
)

// ErrInvalidTimelineItemID denotes an invalid timeline item id; that is not uuid.
var ErrInvalidTimelineItemID = InvalidArgumentError("invalid timeline item ID")

// TimelineItem model.
type TimelineItem struct {
	ID     string `json:"id"`
	UserID string `json:"-"`
	PostID string `json:"-"`
	Post   *Post  `json:"post,omitempty"`
}

// CreateTimelineItem publishes a post to the user timeline and fan-outs it to his followers.
func (s *Service) CreateTimelineItem(ctx context.Context, content string, spoilerOf *string, nsfw bool) (TimelineItem, error) {
	var ti TimelineItem
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return ti, ErrUnauthenticated
	}

	content = smartTrim(content)
	if content == "" || utf8.RuneCountInString(content) > postContentMaxLength {
		return ti, ErrInvalidContent
	}

	if spoilerOf != nil {
		*spoilerOf = smartTrim(*spoilerOf)
		if *spoilerOf == "" || utf8.RuneCountInString(*spoilerOf) > postSpoilerMaxLength {
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

type Timeline []TimelineItem

func (tt Timeline) EndCursor() *string {
	if len(tt) == 0 {
		return nil
	}

	last := tt[len(tt)-1]
	if last.Post == nil {
		return nil
	}

	return strPtr(encodeCursor(last.Post.ID, last.Post.CreatedAt))
}

// Timeline of the authenticated user in descending order and with backward pagination.
func (s *Service) Timeline(ctx context.Context, last uint64, before *string) (Timeline, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, ErrUnauthenticated
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

	last = normalizePageSize(last)
	query, args, err := buildQuery(`
		SELECT timeline.id
		, posts.id
		, posts.content
		, posts.spoiler_of
		, posts.nsfw
		, posts.reactions
		, reactions.user_reactions
		, posts.comments_count
		, posts.created_at
		, posts.user_id = @uid AS post_mine
		, subscriptions.user_id IS NOT NULL AS post_subscribed
		, users.username
		, users.avatar
		FROM timeline
		INNER JOIN posts ON timeline.post_id = posts.id
		INNER JOIN users ON posts.user_id = users.id
		LEFT JOIN (
			SELECT user_id
			, post_id
			, json_agg(json_build_object('reaction', reaction, 'type', type)) AS user_reactions
			FROM post_reactions
			GROUP BY user_id, post_id
		) AS reactions ON reactions.user_id = @uid AND reactions.post_id = posts.id
		LEFT JOIN post_subscriptions AS subscriptions
			ON subscriptions.user_id = @uid AND subscriptions.post_id = posts.id
		WHERE timeline.user_id = @uid
		{{ if and .beforePostID .beforeCreatedAt }}
			AND posts.created_at <= @beforeCreatedAt
			AND (
				posts.id < @beforePostID
					OR posts.created_at < @beforeCreatedAt
			)
		{{ end }}
		ORDER BY posts.created_at DESC, posts.id ASC
		LIMIT @last`, map[string]interface{}{
		"uid":             uid,
		"last":            last,
		"beforePostID":    beforePostID,
		"beforeCreatedAt": beforeCreatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build timeline sql query: %w", err)
	}

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select timeline: %w", err)
	}

	defer rows.Close()

	var tt Timeline
	for rows.Next() {
		var ti TimelineItem
		var p Post
		var rawReactions []byte
		var rawUserReactions []byte
		var u User
		var avatar sql.NullString
		if err = rows.Scan(
			&ti.ID,
			&p.ID,
			&p.Content,
			&p.SpoilerOf,
			&p.NSFW,
			&rawReactions,
			&rawUserReactions,
			&p.CommentsCount,
			&p.CreatedAt,
			&p.Mine,
			&p.Subscribed,
			&u.Username,
			&avatar,
		); err != nil {
			return nil, fmt.Errorf("could not scan timeline item: %w", err)
		}

		if rawReactions != nil {
			err = json.Unmarshal(rawReactions, &p.Reactions)
			if err != nil {
				return nil, fmt.Errorf("could not json unmarshall timeline post reactions: %w", err)
			}
		}

		if rawUserReactions != nil {
			var userReactions []userReaction
			err = json.Unmarshal(rawUserReactions, &userReactions)
			if err != nil {
				return nil, fmt.Errorf("could not json unmarshall user timeline post reactions: %w", err)
			}

			for i, r := range p.Reactions {
				var reacted bool
				for _, ur := range userReactions {
					if r.Type == ur.Type && r.Reaction == ur.Reaction {
						reacted = true
						break
					}
				}
				p.Reactions[i].Reacted = &reacted
			}
		}

		u.AvatarURL = s.avatarURL(avatar)
		p.User = &u
		ti.Post = &p
		tt = append(tt, ti)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not iterate timeline rows: %w", err)
	}

	return tt, nil
}

// TimelineItemStream to receive timeline items in realtime.
func (s *Service) TimelineItemStream(ctx context.Context) (<-chan TimelineItem, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, ErrUnauthenticated
	}

	tt := make(chan TimelineItem)
	unsub, err := s.PubSub.Sub(timelineTopic(uid), func(data []byte) {
		go func(r io.Reader) {
			var ti TimelineItem
			err := gob.NewDecoder(r).Decode(&ti)
			if err != nil {
				_ = s.Logger.Log("error", fmt.Errorf("could not gob decode timeline item: %w", err))
				return
			}

			tt <- ti
		}(bytes.NewReader(data))
	})
	if err != nil {
		return nil, fmt.Errorf("could not subscribe to timeline: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := unsub(); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not unsubcribe from timeline: %w", err))
			// don't return
		}

		close(tt)
	}()

	return tt, nil
}

// DeleteTimelineItem from the auth user timeline.
func (s *Service) DeleteTimelineItem(ctx context.Context, timelineItemID string) error {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return ErrUnauthenticated
	}

	if !reUUID.MatchString(timelineItemID) {
		return ErrInvalidTimelineItemID
	}

	if _, err := s.DB.ExecContext(ctx, `
		DELETE FROM timeline
		WHERE id = $1 AND user_id = $2`, timelineItemID, uid); err != nil {
		return fmt.Errorf("could not sql delete timeline item: %w", err)
	}

	return nil
}

func (s *Service) fanoutPost(p Post) {
	query := `
		INSERT INTO timeline (user_id, post_id)
		SELECT follower_id, $1 FROM follows WHERE followee_id = $2
		RETURNING id, user_id`
	rows, err := s.DB.Query(query, p.ID, p.UserID)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not insert timeline: %w", err))
		return
	}

	defer rows.Close()

	for rows.Next() {
		var ti TimelineItem
		if err = rows.Scan(&ti.ID, &ti.UserID); err != nil {
			_ = s.Logger.Log("error", fmt.Errorf("could not scan timeline item: %w", err))
			return
		}

		ti.PostID = p.ID
		ti.Post = &p

		go s.broadcastTimelineItem(ti)
	}

	if err = rows.Err(); err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not iterate timeline rows: %w", err))
		return
	}
}

func (s *Service) broadcastTimelineItem(ti TimelineItem) {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(ti)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not gob encode timeline item: %w", err))
		return
	}

	err = s.PubSub.Pub(timelineTopic(ti.UserID), b.Bytes())
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not publish timeline item: %w", err))
		return
	}
}

func timelineTopic(userID string) string { return "timeline_item_" + userID }
