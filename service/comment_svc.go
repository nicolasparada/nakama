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

	out, err := svc.Cockroach.CreateComment(ctx, in)
	if err != nil {
		return out, err
	}

	if mentions := extractMentions(in.Content); len(mentions) != 0 {
		svc.background(func(ctx context.Context) error {
			return svc.Cockroach.CreateMentionNotifications(ctx, types.CreateMentionNotifications{
				ActorUserID:    loggedInUser.ID,
				NotifiableKind: types.NotifiableKindComment,
				NotifiableID:   out.ID,
				Usernames:      mentions,
			})
		})
	}

	return out, nil
}

func (svc *Service) Comments(ctx context.Context, postID string) (types.Page[types.Comment], error) {
	var out types.Page[types.Comment]

	if !id.Valid(postID) {
		return out, errs.NewInvalidArgumentError("PostID", "Invalid post ID")
	}

	return svc.Cockroach.Comments(ctx, postID)
}
