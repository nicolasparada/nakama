package service

import (
	"context"

	"github.com/nakamauwu/nakama/types"
)

func (svc *Service) LoginFromProvider(ctx context.Context, in types.ProvidedUser) (types.User, error) {
	var user types.User

	if err := in.Validate(); err != nil {
		return user, err
	}

	user, err := svc.Cockroach.CreateUserWithProvider(ctx, in)
	if err != nil {
		return user, err
	}

	user.SetAvatarURL(svc.MinioBaseURL, AvatarsBucket)

	return user, nil
}
