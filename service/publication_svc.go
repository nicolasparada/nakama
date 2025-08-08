package service

import (
	"context"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) CreatePublication(ctx context.Context, in types.CreatePublication) (types.Created, error) {
	var out types.Created

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	in.SetUserID(loggedInUser.ID)

	return svc.Cockroach.CreatePublication(ctx, in)
}

func (svc *Service) Publications(ctx context.Context, in types.ListPublications) (types.Page[types.Publication], error) {
	var out types.Page[types.Publication]

	if err := in.Validate(); err != nil {
		return out, err
	}

	return svc.Cockroach.Publications(ctx, in)
}

func (svc *Service) Publication(ctx context.Context, mangaID string) (types.Publication, error) {
	var out types.Publication

	if !id.Valid(mangaID) {
		return out, errs.NewInvalidArgumentError("MangaID", "Invalid manga ID")
	}

	return svc.Cockroach.Publication(ctx, mangaID)
}
