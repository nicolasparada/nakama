package nakama

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cockroachdb/cockroach-go/crdb"
	"github.com/lib/pq"
)

type PostReaction struct {
	Emoji  *string `json:"emoji"`
	Custom *string `json:"custom"`
	Count  uint64  `json:"count"`
}

func (s *Service) AddPostReaction(ctx context.Context, postID, emoji string) ([]PostReaction, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, ErrUnauthenticated
	}

	if !reUUID.MatchString(postID) {
		return nil, ErrInvalidPostID
	}

	// TODO: validate emoji.

	var out []PostReaction
	err := crdb.ExecuteTx(ctx, s.DB, nil, func(tx *sql.Tx) error {
		var rr []PostReaction
		query := "SELECT reaction_counts FROM posts WHERE posts.id = $1"
		row := tx.QueryRowContext(ctx, query, postID)
		err := row.Scan(pq.Array(&rr))
		if err != nil {
			return fmt.Errorf("could not sql scan post reaction counts: %w", err)
		}

		query = "INSERT INTO post_reactions (user_id, post_id, emoji) VALUES ($1, $2, $3)"
		_, err = tx.ExecContext(ctx, query, uid, postID, emoji)
		if isUniqueViolation(err) {
			out = rr
			return nil
		}

		if err != nil {
			return fmt.Errorf("could not sql insert post reaction: %w", err)
		}

		var updated bool
		for i, r := range rr {
			if r.Emoji != nil && *r.Emoji == emoji {
				rr[i].Count++
				updated = true
			}
		}

		if !updated {
			rr = append(rr, PostReaction{
				Emoji: &emoji,
				Count: 1,
			})
		}

		query = "UPDATE posts SET reaction_counts = $1 WHERE posts.id = $1"
		_, err = tx.ExecContext(ctx, query, pq.Array(rr), postID)
		if err != nil {
			return fmt.Errorf("could not sql update post reaction counts: %w", err)
		}

		out = rr

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}
