package nakama

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/cockroach-go/crdb"
)

type Reaction struct {
	Type     string `json:"type"`
	Reaction string `json:"reaction"`
	Count    uint64 `json:"count"`
	Reacted  bool   `json:"reacted"`
}

func (s *Service) AddPostReaction(ctx context.Context, postID, emoji string) ([]Reaction, error) {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return nil, ErrUnauthenticated
	}

	if !reUUID.MatchString(postID) {
		return nil, ErrInvalidPostID
	}

	// TODO: validate emoji.

	var out []Reaction
	err := crdb.ExecuteTx(ctx, s.DB, nil, func(tx *sql.Tx) error {
		var b []byte
		query := "SELECT reaction_counts FROM posts WHERE posts.id = $1"
		row := tx.QueryRowContext(ctx, query, postID)
		err := row.Scan(&b)
		if err != nil {
			return fmt.Errorf("could not sql scan post reaction counts: %w", err)
		}

		var rr []Reaction
		if b != nil {
			err = json.Unmarshal(b, &rr)
			if err != nil {
				return fmt.Errorf("could not json unmarshall post reaction counts: %w", err)
			}
		}

		query = "INSERT INTO post_reactions (user_id, post_id, reaction, type) VALUES ($1, $2, $3, $4)"
		_, err = tx.ExecContext(ctx, query, uid, postID, emoji, "emoji")
		if isUniqueViolation(err) {
			out = rr
			return ErrAlreadyExists
		}

		if err != nil {
			return fmt.Errorf("could not sql insert post reaction: %w", err)
		}

		var updated bool
		for i, r := range rr {
			if r.Type == "emoji" && r.Reaction == emoji {
				rr[i].Count++
				updated = true
			}
		}

		if !updated {
			rr = append(rr, Reaction{
				Reaction: emoji,
				Type:     "emoji",
				Count:    1,
			})
		}

		b, err = json.Marshal(rr)
		if err != nil {
			return fmt.Errorf("could not json marshall post reaction counts: %w", err)
		}

		query = "UPDATE posts SET reaction_counts = $1 WHERE posts.id = $2"
		_, err = tx.ExecContext(ctx, query, b, postID)
		if err != nil {
			return fmt.Errorf("could not sql update post reaction counts: %w", err)
		}

		out = rr

		return nil
	})
	if err != nil && err != ErrAlreadyExists {
		return nil, err
	}

	return out, nil
}
