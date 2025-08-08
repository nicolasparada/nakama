package auth

import (
	"context"

	"github.com/nicolasparada/nakama/types"
)

var ctxKeyUser = struct{ name string }{name: "ctx-key-user"}

func ContextWithUser(ctx context.Context, user types.User) context.Context {
	return context.WithValue(ctx, ctxKeyUser, user)
}

func UserFromContext(ctx context.Context) (types.User, bool) {
	user, ok := ctx.Value(ctxKeyUser).(types.User)
	return user, ok
}
