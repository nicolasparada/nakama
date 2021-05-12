package handler

import (
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/securecookie"
	"github.com/matryer/way"
	"github.com/nicolasparada/nakama/storage"
	"github.com/nicolasparada/nakama/transport"
)

type handler struct {
	svc              transport.Service
	logger           log.Logger
	store            storage.Store
	cookieCodec      *securecookie.SecureCookie
	embedStaticFiles bool
}

// New makes use of the service to provide an http.Handler with predefined routing.
func New(svc transport.Service, logger log.Logger, store storage.Store, cdc *securecookie.SecureCookie, embedStaticFiles bool) http.Handler {
	h := &handler{
		svc:              svc,
		logger:           logger,
		store:            store,
		cookieCodec:      cdc,
		embedStaticFiles: embedStaticFiles,
	}

	api := way.NewRouter()
	api.HandleFunc("POST", "/api/send_magic_link", h.sendMagicLink)
	api.HandleFunc("GET", "/api/verify_magic_link", h.verifyMagicLink)
	api.HandleFunc("GET", "/api/credential_creation_options", h.credentialCreationOptions)
	api.HandleFunc("POST", "/api/credentials", h.registerCredential)
	api.HandleFunc("GET", "/api/credential_request_options", h.credentialRequestOptions)
	api.HandleFunc("POST", "/api/webauthn_login", h.webAuthnLogin)
	api.HandleFunc("POST", "/api/dev_login", h.devLogin)
	api.HandleFunc("GET", "/api/auth_user", h.authUser)
	api.HandleFunc("GET", "/api/token", h.token)
	api.HandleFunc("GET", "/api/users", h.users)
	api.HandleFunc("GET", "/api/usernames", h.usernames)
	api.HandleFunc("GET", "/api/users/:username", h.user)
	api.HandleFunc("PATCH", "/api/auth_user", h.updateUser)
	api.HandleFunc("PUT", "/api/auth_user/avatar", h.updateAvatar)
	api.HandleFunc("POST", "/api/users/:username/toggle_follow", h.toggleFollow)
	api.HandleFunc("GET", "/api/users/:username/followers", h.followers)
	api.HandleFunc("GET", "/api/users/:username/followees", h.followees)
	api.HandleFunc("POST", "/api/posts", h.createPost)
	api.HandleFunc("GET", "/api/users/:username/posts", h.posts)
	api.HandleFunc("GET", "/api/posts/:post_id", h.post)
	api.HandleFunc("DELETE", "/api/posts/:post_id", h.deletePost)
	api.HandleFunc("POST", "/api/posts/:post_id/toggle_like", h.togglePostLike)
	api.HandleFunc("POST", "/api/posts/:post_id/add_reaction", h.addPostReaction)
	api.HandleFunc("POST", "/api/posts/:post_id/toggle_subscription", h.togglePostSubscription)
	api.HandleFunc("GET", "/api/timeline", h.timeline)
	api.HandleFunc("DELETE", "/api/timeline/:timeline_item_id", h.deleteTimelineItem)
	api.HandleFunc("POST", "/api/posts/:post_id/comments", h.createComment)
	api.HandleFunc("GET", "/api/posts/:post_id/comments", h.comments)
	api.HandleFunc("POST", "/api/comments/:comment_id/toggle_like", h.toggleCommentLike)
	api.HandleFunc("GET", "/api/notifications", h.notifications)
	api.HandleFunc("GET", "/api/has_unread_notifications", h.hasUnreadNotifications)
	api.HandleFunc("POST", "/api/notifications/:notification_id/mark_as_read", h.markNotificationAsRead)
	api.HandleFunc("POST", "/api/mark_notifications_as_read", h.markNotificationsAsRead)
	api.HandleFunc("HEAD", "/api/proxy", withCacheControl(proxyCacheControl)(h.proxy))

	r := way.NewRouter()
	r.Handle("*", "/api/...", h.withAuth(api))
	r.HandleFunc("GET", "/img/avatars/:name", h.avatar)
	r.Handle("GET", "/...", h.staticHandler())

	return r
}
