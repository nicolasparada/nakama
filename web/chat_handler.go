package web

import (
	"fmt"
	"net/http"

	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/types"
	"golang.org/x/sync/errgroup"
)

func (h *Handler) showChats(w http.ResponseWriter, r *http.Request) {
	pageArgs, err := parsePageArgs(r.URL.Query())
	if err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("parse page args: %w", err))
		return
	}

	ctx := r.Context()
	chats, err := h.Service.Chats(ctx, types.ListChats{PageArgs: pageArgs})
	if err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("fetch chats: %w", err))
		return
	}

	h.render(w, r, "chats.tmpl", map[string]any{
		"Chats": chats,
	}, http.StatusOK)
}

func (h *Handler) chatRedirect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	ctx := r.Context()
	userID := q.Get("user_id")
	chat, err := h.Service.ChatFromParticipants(ctx, types.RetrieveChatFromParticipants{
		OtherUserID: userID,
	})
	if errs.IsNotFound(err) {
		http.Redirect(w, r, "/chats/new?user_id="+userID, http.StatusSeeOther)
		return
	}

	if err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("fetch or create chat from participants: %w", err))
		return
	}

	http.Redirect(w, r, "/chats/"+chat.ID, http.StatusSeeOther)
}

func (h *Handler) showCreateChat(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	userID := q.Get("user_id")

	ctx := r.Context()
	otherUser, err := h.Service.User(ctx, types.RetrieveUser{
		UserID: userID,
	})
	if err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("fetch user: %w", err))
		return
	}

	h.render(w, r, "new_chat.tmpl", map[string]any{
		"OtherUser": otherUser,
	}, http.StatusOK)
}

func (h *Handler) createChat(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseForm(); err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	ctx := r.Context()
	in := types.CreateChat{
		OtherUserID: r.FormValue("user_id"),
		Content:     r.FormValue("message"),
	}

	chat, err := h.Service.CreateChat(ctx, in)
	if err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("create chat: %w", err))
		return
	}

	http.Redirect(w, r, "/chats/"+chat.ID, http.StatusSeeOther)
}

func (h *Handler) showChat(w http.ResponseWriter, r *http.Request) {
	pageArgs, err := parsePageArgs(r.URL.Query())
	if err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("parse page args: %w", err))
		return
	}

	var (
		chat     types.Chat
		messages types.Page[types.Message]
	)

	ctx := r.Context()
	chatID := r.PathValue("chatID")
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		chat, err = h.Service.Chat(gctx, types.RetrieveChat{
			ChatID: chatID,
		})
		return err
	})

	g.Go(func() error {
		var err error
		messages, err = h.Service.Messages(ctx, types.ListMessages{
			ChatID:   chatID,
			PageArgs: pageArgs,
		})
		return err
	})

	if err := g.Wait(); err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("fetch chat/messages: %w", err))
		return
	}

	h.render(w, r, "chat.tmpl", map[string]any{
		"Chat":     chat,
		"Messages": messages,
	}, http.StatusOK)
}

func (h *Handler) createMessage(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseForm(); err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	ctx := r.Context()
	in := types.CreateMessage{
		ChatID:  r.FormValue("chat_id"),
		Content: r.FormValue("message"),
	}

	_, err := h.Service.CreateMessage(ctx, in)
	if err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("create message: %w", err))
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}
