package service

import (
	"context"
	"fmt"

	"github.com/nicolasparada/go-errs"
)

type ResourceKind string

const (
	ResourceKindPost    ResourceKind = "post"
	ResourceKindComment ResourceKind = "comment"
)

func (svc *Service) authorize(ctx context.Context, resourceKind ResourceKind, resourceID string) error {
	userID, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return errs.Unauthenticated
	}

	var resourceUserID string
	var err error

	switch resourceKind {
	case ResourceKindPost:
		resourceUserID, err = svc.Cockroach.PostUserID(ctx, resourceID)
	case ResourceKindComment:
		resourceUserID, err = svc.Cockroach.CommentUserID(ctx, resourceID)
	default:
		return fmt.Errorf("unknown resource kind %q", resourceKind)
	}

	if err != nil {
		return err
	}

	if resourceUserID != userID {
		return errs.PermissionDenied
	}

	return nil
}
