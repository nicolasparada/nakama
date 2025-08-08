package service

import (
	"context"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) CreateComment(ctx context.Context, in types.CreateComment) (types.Created, error) {
	var out types.Created

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetUserID(loggedInUser.ID)

	return svc.Cockroach.CreateComment(ctx, in)
}

func (svc *Service) Comments(ctx context.Context, postID string) (types.Page[types.Comment], error) {
	var out types.Page[types.Comment]

	if !id.Valid(postID) {
		return out, errs.NewInvalidArgumentError("PostID", "Invalid post ID")
	}

	return svc.Cockroach.Comments(ctx, postID)
}
