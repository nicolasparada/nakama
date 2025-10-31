package cockroach

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

func (c *Cockroach) ChatFromParticipants(ctx context.Context, in types.RetrieveChatFromParticipants) (types.Chat, error) {
	var out types.Chat

	const q = `
		SELECT chats.*
		FROM participants
		INNER JOIN chats ON participants.chat_id = chats.id
		WHERE (participants.user_id = @user_id AND participants.other_user_id = @other_user_id)
	`

	rows, err := c.db.Query(ctx, q, pgx.NamedArgs{
		"user_id":       in.LoggedInUserID(),
		"other_user_id": in.OtherUserID,
	})
	if err != nil {
		return out, fmt.Errorf("sql query chat from participants: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Chat])
	if db.IsNotFoundError(err) {
		return out, errs.NewNotFoundError("chat not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql collect chat from participants: %w", err)
	}

	return out, nil
}

func (c *Cockroach) CreateChat(ctx context.Context, in types.CreateChat) (types.Created, error) {
	var out types.Created
	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		chat, err := c.createChat(ctx)
		if err != nil {
			return err
		}

		in.SetChatID(chat.ID)

		following, err := c.FollowingEachOther(ctx, in.LoggedInUserID(), in.OtherUserID)
		if err != nil {
			return err
		}

		in.SetFollowingEachOther(following)

		if err := c.createParticipants(ctx, in); err != nil {
			return err
		}

		createMessage := types.CreateMessage{
			ChatID:  chat.ID,
			Content: in.Content,
		}
		createMessage.SetLoggedInUserID(in.LoggedInUserID())
		if _, err := c.CreateMessage(ctx, createMessage); err != nil {
			return err
		}

		out = chat

		return nil
	})
}

func (c *Cockroach) createChat(ctx context.Context) (types.Created, error) {
	var out types.Created

	const q = `
		INSERT INTO chats (id)
		VALUES (@chat_id)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"chat_id": id.Generate(),
	})
	if err != nil {
		return out, fmt.Errorf("sql insert chat: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted chat: %w", err)
	}

	return out, nil
}

func (c *Cockroach) createParticipants(ctx context.Context, in types.CreateChat) error {
	senderStatus := types.ParticipantStatusPendingSender
	receiverStatus := types.ParticipantStatusPendingReceiver

	if in.FollowingEachOther() {
		senderStatus = types.ParticipantStatusActive
		receiverStatus = types.ParticipantStatusActive
	}

	const q = `
		INSERT INTO participants (user_id, chat_id, status, other_user_id)
		VALUES (@user_id, @chat_id, @sender_status, @other_user_id)
			 , (@other_user_id, @chat_id, @receiver_status, @user_id)
	`

	_, err := c.db.Exec(ctx, q, pgx.StrictNamedArgs{
		"user_id":         in.LoggedInUserID(),
		"chat_id":         in.ChatID(),
		"sender_status":   senderStatus,
		"receiver_status": receiverStatus,
		"other_user_id":   in.OtherUserID,
	})
	if err != nil {
		return fmt.Errorf("sql insert participants: %w", err)
	}

	return nil
}

func (c *Cockroach) CreateMessage(ctx context.Context, in types.CreateMessage) (types.Created, error) {
	var out types.Created
	return out, c.db.RunTx(ctx, func(ctx context.Context) error {
		senderStatus, err := c.senderChatStatus(ctx, in.ChatID, in.LoggedInUserID())
		if err != nil {
			return err
		}

		// Check if sender is trying to send a message when they should wait for a reply
		if senderStatus == types.ParticipantStatusPendingSender {
			return errs.NewPermissionDeniedError("cannot send message: waiting for the other user to reply or accept the conversation")
		}

		// If receiver is replying for the first time, activate the conversation
		if senderStatus == types.ParticipantStatusPendingReceiver {
			if err := c.activateParticipants(ctx, in.ChatID); err != nil {
				return fmt.Errorf("activate conversation: %w", err)
			}
		}

		msg, err := c.createMessage(ctx, in)
		if err != nil {
			return err
		}

		// Mark as unread for the recipient
		if err := c.markChatAsUnreadForRecipient(ctx, in.ChatID, in.LoggedInUserID()); err != nil {
			return fmt.Errorf("mark as unread for recipient: %w", err)
		}

		out = msg
		return nil
	})
}

func (c *Cockroach) createMessage(ctx context.Context, in types.CreateMessage) (types.Created, error) {
	var out types.Created

	const q = `
		INSERT INTO messages (id, user_id, chat_id, content)
		VALUES (@message_id, @user_id, @chat_id, @content)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.NamedArgs{
		"message_id": id.Generate(),
		"user_id":    in.LoggedInUserID(),
		"chat_id":    in.ChatID,
		"content":    in.Content,
	})
	if err != nil {
		return out, fmt.Errorf("sql insert message: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted message: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Chat(ctx context.Context, in types.RetrieveChat) (types.Chat, error) {
	var out types.Chat

	const q = `
		SELECT chats.*,
			json_build_object(
				'userID', participants.user_id,
				'chatID', participants.chat_id,
				'otherUserID', participants.other_user_id,
				'status', participants.status,
				'hasUnread', participants.has_unread,
				'lastReadAt', participants.last_read_at,
				'lastActivityAt', participants.last_activity_at,
				'createdAt', participants.created_at,
				'updatedAt', participants.updated_at,
				'otherUser', json_build_object(
					'id', other_user.id,
					'username', other_user.username,
					'avatar', other_user.avatar
				)
			) AS participation
		FROM chats
		INNER JOIN participants ON participants.chat_id = chats.id
		INNER JOIN users AS other_user ON other_user.id = participants.other_user_id
		WHERE chats.id = @chat_id
				AND participants.user_id = @user_id
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"chat_id": in.ChatID,
		"user_id": in.LoggedInUserID(),
	})
	if err != nil {
		return out, fmt.Errorf("sql select chat: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Chat])
	if db.IsNotFoundError(err) {
		return out, errs.NewNotFoundError("chat not found")
	}

	if err != nil {
		return out, fmt.Errorf("sql collect chat: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Chats(ctx context.Context, in types.ListChats) (types.Page[types.Chat], error) {
	var out types.Page[types.Chat]

	query := `
		SELECT chats.*,
			json_build_object(
				'userID', participants.user_id,
				'chatID', participants.chat_id,
				'otherUserID', participants.other_user_id,
				'status', participants.status,
				'hasUnread', participants.has_unread,
				'lastReadAt', participants.last_read_at,
				'lastActivityAt', participants.last_activity_at,
				'createdAt', participants.created_at,
				'updatedAt', participants.updated_at,
				'otherUser', json_build_object(
					'id', other_user.id,
					'username', other_user.username,
					'avatar', other_user.avatar
				)
			) AS participation
		FROM chats
		INNER JOIN participants ON participants.chat_id = chats.id
		INNER JOIN users AS other_user ON other_user.id = participants.other_user_id
		WHERE participants.user_id = @user_id
	`
	args := pgx.StrictNamedArgs{
		"user_id": in.LoggedInUserID(),
	}

	query = addPageFilter(query, "chats", args, in.PageArgs)
	query = addPageOrder(query, "chats", in.PageArgs)
	query = addPageLimit(query, args, in.PageArgs)

	rows, err := c.db.Query(ctx, query, args)
	if err != nil {
		return out, fmt.Errorf("sql select chats: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Chat])
	if err != nil {
		return out, fmt.Errorf("sql collect chats: %w", err)
	}

	applyPageInfo(&out, in.PageArgs, func(c types.Chat) string { return c.ID })

	return out, nil
}

func (c *Cockroach) senderChatStatus(ctx context.Context, chatID, userID string) (types.ParticipantStatus, error) {
	var status types.ParticipantStatus

	const q = `
		SELECT status
		FROM participants
		WHERE chat_id = @chat_id
			AND user_id = @user_id
	`

	err := c.db.QueryRow(ctx, q, pgx.StrictNamedArgs{
		"chat_id": chatID,
		"user_id": userID,
	}).Scan(&status)
	if db.IsNotFoundError(err) {
		return status, errs.NewNotFoundError("participant not found")
	}

	if err != nil {
		return status, fmt.Errorf("sql select sender chat status: %w", err)
	}

	return status, nil
}

func (c *Cockroach) activateParticipants(ctx context.Context, chatID string) error {
	const q = `
		UPDATE participants 
		SET status = @active_status,
			updated_at = NOW()
		WHERE chat_id = @chat_id
			AND status IN (@pending_sender, @pending_receiver)
	`

	_, err := c.db.Exec(ctx, q, pgx.StrictNamedArgs{
		"chat_id":          chatID,
		"active_status":    types.ParticipantStatusActive,
		"pending_sender":   types.ParticipantStatusPendingSender,
		"pending_receiver": types.ParticipantStatusPendingReceiver,
	})
	if err != nil {
		return fmt.Errorf("sql update participants to active: %w", err)
	}

	return nil
}

func (c *Cockroach) markChatAsUnreadForRecipient(ctx context.Context, chatID, senderUserID string) error {
	const q = `
		UPDATE participants 
		SET has_unread = true,
			last_activity_at = NOW(),
			updated_at = NOW()
		WHERE chat_id = @chat_id
			AND user_id != @sender_user_id
	`

	_, err := c.db.Exec(ctx, q, pgx.StrictNamedArgs{
		"chat_id":        chatID,
		"sender_user_id": senderUserID,
	})
	if err != nil {
		return fmt.Errorf("sql update recipient unread status: %w", err)
	}

	return nil
}

func (c *Cockroach) markChatAsRead(ctx context.Context, chatID, userID string) error {
	const q = `
		UPDATE participants 
		SET has_unread = false,
			last_read_at = NOW(),
			updated_at = NOW()
		WHERE chat_id = @chat_id
			AND user_id = @user_id
	`

	_, err := c.db.Exec(ctx, q, pgx.StrictNamedArgs{
		"chat_id": chatID,
		"user_id": userID,
	})
	if err != nil {
		return fmt.Errorf("sql update messages as read: %w", err)
	}

	return nil
}
