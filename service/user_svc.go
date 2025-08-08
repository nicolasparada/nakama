package service

import (
	"context"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) UpsertUser(ctx context.Context, in types.UpsertUser) (types.User, error) {
	var out types.User

	if err := in.Validate(); err != nil {
		return out, err
	}

	return svc.Cockroach.UpsertUser(ctx, in)
}

func (svc *Service) User(ctx context.Context, in types.RetrieveUser) (types.User, error) {
	var out types.User

	if err := in.Validate(); err != nil {
		return out, err
	}

	if u, loggedIn := auth.UserFromContext(ctx); loggedIn {
		in.SetLoggedInUserID(u.ID)
	}

	return svc.Cockroach.User(ctx, in)
}

func (svc *Service) UserFromUsername(ctx context.Context, in types.RetrieveUserFromUsername) (types.User, error) {
	var out types.User

	if err := in.Validate(); err != nil {
		return out, err
	}

	if u, loggedIn := auth.UserFromContext(ctx); loggedIn {
		in.SetLoggedInUserID(u.ID)
	}

	return svc.Cockroach.UserFromUsername(ctx, in)
}

func (svc *Service) ToggleFollow(ctx context.Context, in types.ToggleFollow) error {
	if err := in.Validate(); err != nil {
		return err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return errs.Unauthenticated
	}

	in.SetLoggedInUserID(loggedInUser.ID)

	if loggedInUser.ID == in.FolloweeID {
		return errs.NewPermissionDeniedError("Cannot follow yourself")
	}

	return svc.Cockroach.ToggleFollow(ctx, in)
}
