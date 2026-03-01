package http

import (
	"net/http"
	"net/url"

	"github.com/go-kit/log"
	"github.com/gorilla/securecookie"
	"github.com/matryer/way"

	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/storage"
)

type handler struct {
	svc              *service.Service
	origin           *url.URL
	logger           log.Logger
	store            storage.Store
	cookieCodec      *securecookie.SecureCookie
	embedStaticFiles bool
}

// New makes use of the service to provide an http.Handler with predefined routing.
func New(svc *service.Service, oauthProviders []OauthProvider, origin *url.URL, logger log.Logger, store storage.Store, cdc *securecookie.SecureCookie, promHandler http.Handler, embedStaticFiles bool) http.Handler {
	h := &handler{
		svc:              svc,
		origin:           origin,
		logger:           logger,
		store:            store,
		cookieCodec:      cdc,
		embedStaticFiles: embedStaticFiles,
	}

	api := way.NewRouter()
	api.HandleFunc("POST", "/api/send_magic_link", h.sendMagicLink)
	api.HandleFunc("GET", "/api/verify_magic_link", h.verifyMagicLink)

	for _, provider := range oauthProviders {
		api.HandleFunc("GET", "/api/"+provider.Name+"_auth", h.oauth2Handler(provider))
		api.HandleFunc("GET", "/api/"+provider.Name+"_auth/callback", h.oauth2CallbackHandler(provider))
	}

	api.HandleFunc("POST", "/api/dev_login", h.devLogin)
	api.HandleFunc("GET", "/api/auth_user", h.authUser)
	api.HandleFunc("GET", "/api/token", h.token)
	api.HandleFunc("GET", "/api/users", h.users)
	api.HandleFunc("GET", "/api/usernames", h.usernames)
	api.HandleFunc("GET", "/api/users/:username", h.user)
	api.HandleFunc("PATCH", "/api/auth_user", h.updateUser)
	api.HandleFunc("PUT", "/api/auth_user/avatar", h.updateAvatar)
	api.HandleFunc("PUT", "/api/auth_user/cover", h.updateCover)
	api.HandleFunc("POST", "/api/users/:username/toggle_follow", h.toggleFollow)
	api.HandleFunc("GET", "/api/users/:username/followers", h.followers)
	api.HandleFunc("GET", "/api/users/:username/followees", h.followees)
	api.HandleFunc("GET", "/api/users/:username/posts", h.posts)
	api.HandleFunc("GET", "/api/posts", h.posts)
	api.HandleFunc("GET", "/api/posts/:post_id", h.post)
	api.HandleFunc("PATCH", "/api/posts/:post_id", h.updatePost)
	api.HandleFunc("DELETE", "/api/posts/:post_id", h.deletePost)
	api.HandleFunc("POST", "/api/posts/:post_id/toggle_reaction", h.togglePostReaction)
	api.HandleFunc("POST", "/api/posts/:post_id/toggle_subscription", h.togglePostSubscription)
	api.HandleFunc("POST", "/api/timeline", h.createTimelineItem)
	api.HandleFunc("GET", "/api/timeline", h.timeline)
	api.HandleFunc("DELETE", "/api/timeline/:timeline_item_id", h.deleteTimelineItem)
	api.HandleFunc("POST", "/api/posts/:post_id/comments", h.createComment)
	api.HandleFunc("GET", "/api/posts/:post_id/comments", h.comments)
	api.HandleFunc("PATCH", "/api/comments/:comment_id", h.updateComment)
	api.HandleFunc("DELETE", "/api/comments/:comment_id", h.deleteComment)
	api.HandleFunc("POST", "/api/comments/:comment_id/toggle_reaction", h.toggleCommentReaction)
	api.HandleFunc("GET", "/api/notifications", h.notifications)
	api.HandleFunc("GET", "/api/has_unread_notifications", h.hasUnreadNotifications)
	api.HandleFunc("POST", "/api/notifications/:notification_id/mark_as_read", h.markNotificationAsRead)
	api.HandleFunc("POST", "/api/mark_notifications_as_read", h.markNotificationsAsRead)
	api.HandleFunc("POST", "/api/web_push_subscriptions", h.addWebPushSubscription)

	proxy := withCacheControl(proxyCacheControl)(h.proxy)
	api.HandleFunc("HEAD", "/api/proxy", proxy)
	api.HandleFunc("GET", "/api/proxy", proxy)

	api.HandleFunc("POST", "/api/logs", h.pushLog)
	api.Handle("GET", "/api/prom", promHandler)

	r := way.NewRouter()
	r.Handle("*", "/api/...", h.withAuth(api))
	r.HandleFunc("GET", "/img/avatars/:name", h.avatar)
	r.HandleFunc("GET", "/img/covers/:name", h.cover)
	r.HandleFunc("GET", "/img/media/:name", h.media)
	r.Handle("GET", "/...", h.staticHandler())

	return r
}
