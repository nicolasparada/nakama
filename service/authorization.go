package service

import (
	"context"
	"fmt"

	"github.com/nicolasparada/go-errs"
)

type ResourceKind string

const (
	ResourceKindComment ResourceKind = "comment"
)

func (svc *Service) authorize(ctx context.Context, resourceKind ResourceKind, resourceID string) error {
	uid, ok := ctx.Value(KeyAuthUserID).(string)
	if !ok {
		return errs.Unauthenticated
	}

	var resourceUserID string
	var err error

	switch resourceKind {
	case ResourceKindComment:
		resourceUserID, err = svc.Cockroach.CommentUserID(ctx, resourceID)
	default:
		return fmt.Errorf("unknown resource kind: %s", resourceKind)
	}

	if err != nil {
		return err
	}

	if resourceUserID != uid {
		return errs.PermissionDenied
	}

	return nil
}
