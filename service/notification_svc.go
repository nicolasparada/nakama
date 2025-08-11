package service

import (
	"context"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) Notifications(ctx context.Context, in types.ListNotifications) (types.Page[types.Notification], error) {
	var out types.Page[types.Notification]

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetUserID(loggedInUser.ID)

	return svc.Cockroach.Notifications(ctx, in)
}

func (svc *Service) ReadNotification(ctx context.Context, in types.ReadNotification) error {
	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return errs.Unauthenticated
	}

	in.SetUserID(loggedInUser.ID)

	return svc.Cockroach.ReadNotification(ctx, in)
}
