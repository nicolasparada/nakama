package service

import (
	"context"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) Messages(ctx context.Context, in types.ListMessages) (types.Page[types.Message], error) {
	var out types.Page[types.Message]

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	return svc.Cockroach.Messages(ctx, in)
}

func (svc *Service) CreateMessage(ctx context.Context, in types.CreateMessage) (types.Created, error) {
	var out types.Created

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	// TODO: check whether can create message in this chat

	return svc.Cockroach.CreateMessage(ctx, in)
}
