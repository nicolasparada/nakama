package cockroach

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/nicolasparada/nakama/types"
)

func (c *Cockroach) Messages(ctx context.Context, in types.ListMessages) (types.Page[types.Message], error) {
	var out types.Page[types.Message]
	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		msgs, err := c.messages(ctx, in)
		if err != nil {
			return err
		}

		// Mark messages as read for this user
		if err := c.markChatAsRead(ctx, in.ChatID, in.LoggedInUserID()); err != nil {
			return fmt.Errorf("mark messages as read: %w", err)
		}

		out = msgs
		return nil
	})
}

func (c *Cockroach) messages(ctx context.Context, in types.ListMessages) (types.Page[types.Message], error) {
	var out types.Page[types.Message]

	query := `
		SELECT messages.*,
			to_json(users) AS user,
			json_build_object(
				'isMine', messages.user_id = @user_id
			) AS relationship
		FROM messages
		INNER JOIN users ON messages.user_id = users.id
		WHERE messages.chat_id = @chat_id
	`
	args := pgx.StrictNamedArgs{
		"chat_id": in.ChatID,
		"user_id": in.LoggedInUserID(),
	}

	query = addPageFilter(query, "messages", args, in.PageArgs)
	query = addPageOrder(query, "messages", in.PageArgs)
	query = addPageLimit(query, args, in.PageArgs)

	rows, err := c.db.Query(ctx, query, args)
	if err != nil {
		return out, fmt.Errorf("sql select messages: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Message])
	if err != nil {
		return out, fmt.Errorf("sql collect messages: %w", err)
	}

	applyPageInfo(&out, in.PageArgs, func(m types.Message) string { return m.ID })

	return out, nil
}
