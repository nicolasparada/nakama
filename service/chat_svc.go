package service

import (
	"context"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) ChatFromParticipants(ctx context.Context, in types.RetrieveChatFromParticipants) (types.Chat, error) {
	var out types.Chat

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	return svc.Cockroach.ChatFromParticipants(ctx, in)
}

func (svc *Service) CreateChat(ctx context.Context, in types.CreateChat) (types.Created, error) {
	var out types.Created

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	return svc.Cockroach.CreateChat(ctx, in)
}

func (svc *Service) Chat(ctx context.Context, in types.RetrieveChat) (types.Chat, error) {
	var out types.Chat

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	return svc.Cockroach.Chat(ctx, in)
}

func (svc *Service) Chats(ctx context.Context, in types.ListChats) (types.Page[types.Chat], error) {
	var out types.Page[types.Chat]

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	return svc.Cockroach.Chats(ctx, in)
}
