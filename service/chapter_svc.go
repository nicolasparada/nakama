package service

import (
	"context"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

func (svc *Service) CreateChapter(ctx context.Context, in types.CreateChapter) (types.Created, error) {
	var out types.Created

	if err := in.Validate(); err != nil {
		return out, err
	}

	loggedInUser, loggedIn := auth.UserFromContext(ctx)
	if !loggedIn {
		return out, errs.Unauthenticated
	}

	manga, err := svc.Cockroach.Publication(ctx, in.PublicationID)
	if err != nil {
		return out, err
	}

	if manga.UserID != loggedInUser.ID {
		return out, errs.NewPermissionDeniedError("You do not have permission to create a chapter in this publication")
	}

	return svc.Cockroach.CreateChapter(ctx, in)
}

func (svc *Service) Chapters(ctx context.Context, publicationID string) (types.Page[types.Chapter], error) {
	var out types.Page[types.Chapter]

	if !id.Valid(publicationID) {
		return out, errs.NewInvalidArgumentError("PublicationID", "Invalid publication ID")
	}

	return svc.Cockroach.Chapters(ctx, publicationID)
}

func (svc *Service) Chapter(ctx context.Context, chapterID string) (types.Chapter, error) {
	var out types.Chapter

	if !id.Valid(chapterID) {
		return out, errs.NewInvalidArgumentError("ChapterID", "Invalid chapter ID")
	}

	return svc.Cockroach.Chapter(ctx, chapterID)
}

func (svc *Service) LatestChapterNumber(ctx context.Context, publicationID string) (uint32, error) {
	if !id.Valid(publicationID) {
		return 0, errs.NewInvalidArgumentError("PublicationID", "Invalid publication ID")
	}

	return svc.Cockroach.LatestChapterNumber(ctx, publicationID)
}
