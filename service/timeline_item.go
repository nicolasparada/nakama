package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/disintegration/imaging"
	"github.com/go-kit/log/level"
	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"golang.org/x/sync/errgroup"

	"github.com/nakamauwu/nakama/cockroach"
	"github.com/nakamauwu/nakama/cursor"
	"github.com/nakamauwu/nakama/storage"
	"github.com/nakamauwu/nakama/textutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
)

const MediaBucket = "media"

const (
	MaxMediaItemBytes = 5 << 20  // 5MB
	MaxMediaBytes     = 15 << 20 // 15MB
)

var (
	// ErrInvalidTimelineItemID denotes an invalid timeline item id; that is not uuid.
	ErrInvalidTimelineItemID = errs.InvalidArgumentError("invalid timeline item ID")
	// ErrUnsupportedMediaItemFormat denotes an unsupported media item format.
	ErrUnsupportedMediaItemFormat = errs.InvalidArgumentError("unsupported media item format")
	ErrMediaItemTooLarge          = errs.InvalidArgumentError("media item too large")
	ErrMediaTooLarge              = errs.InvalidArgumentError("media too large")
)

// CreateTimelineItem publishes a post to the user timeline and fan-outs it to his followers.
func (s *Service) CreateTimelineItem(ctx context.Context, content string, spoilerOf *string, nsfw bool, media []io.ReadSeeker) (types.TimelineItem, error) {
	var ti types.TimelineItem
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return ti, errs.Unauthenticated
	}

	content = textutil.SmartTrim(content)
	if len(media) == 0 && content == "" || utf8.RuneCountInString(content) > postContentMaxLength {
		return ti, ErrInvalidContent
	}

	if spoilerOf != nil {
		*spoilerOf = textutil.SmartTrim(*spoilerOf)
		if *spoilerOf == "" || utf8.RuneCountInString(*spoilerOf) > postSpoilerMaxLength {
			return ti, ErrInvalidSpoiler
		}
	}

	tags := textutil.CollectTags(content)

	type File struct {
		Name        string
		ContentType string
		Content     []byte
	}

	var files []File

	if len(media) != 0 {
		files = make([]File, len(media))

		g := errgroup.Group{}
		var mu sync.Mutex

		for i, mediaItem := range media {
			i := i
			mediaItem := mediaItem

			g.Go(func() error {
				ct, err := detectContentType(mediaItem)
				if err != nil {
					return fmt.Errorf("create timeline item: detect media content type: %w", err)
				}

				if ct != "image/png" && ct != "image/jpeg" {
					return ErrUnsupportedAvatarFormat
				}

				img, err := imaging.Decode(io.LimitReader(mediaItem, MaxMediaItemBytes), imaging.AutoOrientation(true))
				if err == image.ErrFormat {
					return ErrUnsupportedMediaItemFormat
				}

				if err != nil {
					return fmt.Errorf("could not image decode post media item: %w", err)
				}

				buf := &bytes.Buffer{}
				if ct == "image/png" {
					err = png.Encode(buf, img)
				} else {
					err = jpeg.Encode(buf, img, nil)
				}
				if err != nil {
					return fmt.Errorf("could not encode post media item: %w", err)
				}

				fileName, err := gonanoid.New()
				if err != nil {
					return fmt.Errorf("could not generate media item filename: %w", err)
				}

				if ct == "image/png" {
					fileName += ".png"
				} else {
					fileName += ".jpg"
				}

				mu.Lock()

				files[i] = File{
					Name:        fileName,
					ContentType: ct,
					Content:     buf.Bytes(),
				}

				mu.Unlock()
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return ti, err
		}
	}

	var mediaItemsBytes int64
	var fileNames []string
	for _, file := range files {
		mediaItemsBytes += int64(len(file.Content))
		fileNames = append(fileNames, file.Name)
	}

	if mediaItemsBytes > MaxMediaBytes {
		return ti, ErrMediaTooLarge
	}

	if len(files) != 0 {
		g, gctx := errgroup.WithContext(ctx)
		for _, file := range files {
			file := file
			g.Go(func() error {
				err := s.Store.Store(gctx, MediaBucket, file.Name, file.Content, storage.StoreWithContentType(file.ContentType))
				if err != nil {
					return fmt.Errorf("could not store post media item: %w", err)
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return ti, err
		}
	}

	var p types.Post
	err := cockroach.ExecuteTx(ctx, s.DB, func(tx pgx.Tx) error {
		query := `
			INSERT INTO posts (user_id, content, spoiler_of, nsfw, media) VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at`
		row := tx.QueryRow(ctx, query, uid, content, spoilerOf, nsfw, pq.Array(fileNames))
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
		p.MediaURLs = s.mediaURLs(fileNames)
		p.UpdatedAt = p.CreatedAt

		query = "INSERT INTO post_subscriptions (user_id, post_id) VALUES ($1, $2)"
		if _, err = tx.Exec(ctx, query, uid, p.ID); err != nil {
			return fmt.Errorf("could not insert post subscription: %w", err)
		}

		p.Subscribed = true

		if len(tags) != 0 {
			var values []string
			args := []any{p.ID}
			for i := 0; i < len(tags); i++ {
				values = append(values, fmt.Sprintf("($1, $%d)", i+2))
				args = append(args, tags[i])
			}

			query := `INSERT INTO post_tags (post_id, tag) VALUES ` + strings.Join(values, ", ")
			_, err := tx.Exec(ctx, query, args...)
			if err != nil {
				return fmt.Errorf("could not sql insert post tags: %w", err)
			}
		}

		query = "INSERT INTO timeline (user_id, post_id) VALUES ($1, $2) RETURNING id"
		err = tx.QueryRow(ctx, query, uid, p.ID).Scan(&ti.ID)
		if err != nil {
			return fmt.Errorf("could not insert timeline item: %w", err)
		}

		ti.UserID = uid
		ti.PostID = p.ID
		ti.Post = &p

		return nil
	})
	if err != nil {
		if len(files) != 0 {
			go func() {
				g, gctx := errgroup.WithContext(ctx)
				for _, fileName := range fileNames {
					fileName := fileName
					g.Go(func() error {
						err := s.Store.Delete(gctx, MediaBucket, fileName)
						if err != nil {
							return fmt.Errorf("could not delete post media item: %w", err)
						}
						return nil
					})
				}
				if err := g.Wait(); err != nil {
					_ = level.Error(s.Logger).Log("msg", "could not delete post media items", "err", err)
				}
			}()
		}

		return ti, err
	}

	go s.postCreated(p)

	return ti, nil
}

func (s *Service) postCreated(p types.Post) {
	u, err := s.userByID(context.Background(), p.UserID)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not fetch post user: %w", err))
		return
	}

	p.User = &u
	p.Mine = false
	p.Subscribed = false

	go s.broadcastPost(p)
	go s.fanoutPost(p)
	go s.notifyPostMention(p)
}

// Timeline of the authenticated user in descending order and with backward pagination.
func (s *Service) Timeline(ctx context.Context, last uint64, before *string) (types.Timeline, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, errs.Unauthenticated
	}

	var beforePostID string
	var beforeCreatedAt time.Time

	if before != nil {
		var err error
		beforePostID, beforeCreatedAt, err = cursor.Decode(*before)
		if err != nil || !types.ValidUUIDv4(beforePostID) {
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
		, posts.media
		, posts.created_at
		, posts.updated_at
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
		LIMIT @last`, map[string]any{
		"uid":             uid,
		"last":            last,
		"beforePostID":    beforePostID,
		"beforeCreatedAt": beforeCreatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("could not build timeline sql query: %w", err)
	}

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("could not query select timeline: %w", err)
	}

	defer rows.Close()

	var tt types.Timeline
	for rows.Next() {
		var ti types.TimelineItem
		var p types.Post
		var rawReactions []byte
		var rawUserReactions []byte
		var u types.User
		var avatar sql.NullString
		var media []string
		if err = rows.Scan(
			&ti.ID,
			&p.ID,
			&p.Content,
			&p.SpoilerOf,
			&p.NSFW,
			&rawReactions,
			&rawUserReactions,
			&p.CommentsCount,
			pq.Array(&media),
			&p.CreatedAt,
			&p.UpdatedAt,
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

		p.MediaURLs = s.mediaURLs(media)
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
func (s *Service) TimelineItemStream(ctx context.Context) (<-chan types.TimelineItem, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, errs.Unauthenticated
	}

	tt := make(chan types.TimelineItem)
	unsub, err := s.PubSub.Sub(timelineTopic(uid), func(data []byte) {
		go func(r io.Reader) {
			var ti types.TimelineItem
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
		return errs.Unauthenticated
	}

	if !types.ValidUUIDv4(timelineItemID) {
		return ErrInvalidTimelineItemID
	}

	if _, err := s.DB.Exec(ctx, `
		DELETE FROM timeline
		WHERE id = $1 AND user_id = $2`, timelineItemID, uid); err != nil {
		return fmt.Errorf("could not sql delete timeline item: %w", err)
	}

	return nil
}

func (s *Service) fanoutPost(p types.Post) {
	query := `
		INSERT INTO timeline (user_id, post_id)
		SELECT follower_id, $1 FROM follows WHERE followee_id = $2
		RETURNING id, user_id`
	rows, err := s.DB.Query(context.Background(), query, p.ID, p.UserID)
	if err != nil {
		_ = s.Logger.Log("error", fmt.Errorf("could not insert timeline: %w", err))
		return
	}

	defer rows.Close()

	for rows.Next() {
		var ti types.TimelineItem
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

func (s *Service) broadcastTimelineItem(ti types.TimelineItem) {
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

func (s *Service) mediaURL(mediaItem string) string {
	return s.MediaURLPrefix + mediaItem
}

func (s *Service) mediaURLs(media []string) []string {
	for i, item := range media {
		media[i] = s.mediaURL(item)
	}
	return media
}
