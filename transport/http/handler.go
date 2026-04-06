package http

import (
	"net/http"
	"net/url"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-kit/log"

	"github.com/nakamauwu/nakama/service"
)

type handler struct {
	svc              *service.Service
	sess             *scs.SessionManager
	origin           *url.URL
	logger           log.Logger
	embedStaticFiles bool
}

// New makes use of the service to provide an http.Handler with predefined routing.
func New(svc *service.Service, sessStore scs.Store, origin *url.URL, logger log.Logger, promHandler http.Handler, embedStaticFiles bool) http.Handler {
	sess := scs.New()
	sess.Store = sessStore
	sess.Lifetime = time.Hour * 24 * 7 * 2 // 2 weeks
	sess.Cookie.Secure = origin.Scheme == "https"

	h := &handler{
		svc:              svc,
		sess:             sess,
		origin:           origin,
		logger:           logger,
		embedStaticFiles: embedStaticFiles,
	}

	api := http.NewServeMux()
	api.HandleFunc("POST /api/auth/login/request", h.requestLogin)
	api.HandleFunc("POST /api/auth/login/verify", h.verifyLogin)

	api.HandleFunc("GET /api/auth/{provider}/redirect", h.oauthRedirect)
	api.HandleFunc("GET /api/auth/{provider}/callback", h.oauthCallback)

	api.HandleFunc("POST /api/auth/signup/complete", h.completeSignup)
	api.HandleFunc("POST /api/auth/logout", h.logout)

	api.HandleFunc("POST /api/dev_login", h.devLogin)
	api.HandleFunc("GET /api/user", h.authUser)
	api.HandleFunc("GET /api/users", h.userProfiles)
	api.HandleFunc("GET /api/usernames", h.usernames)
	api.HandleFunc("GET /api/users/{username}", h.userProfileByUsername)
	api.HandleFunc("PATCH /api/user", h.updateUser)
	api.HandleFunc("PUT /api/user/avatar", h.updateAvatar)
	api.HandleFunc("PUT /api/user/cover", h.updateCover)
	api.HandleFunc("POST /api/user/email/request", h.requestEmailUpdate)
	api.HandleFunc("PATCH /api/user/email/verify", h.verifyEmailUpdate)
	api.HandleFunc("POST /api/users/{username}/toggle_follow", h.toggleFollow)
	api.HandleFunc("GET /api/users/{username}/followers", h.followers)
	api.HandleFunc("GET /api/users/{username}/followees", h.followees)
	api.HandleFunc("GET /api/users/{username}/posts", h.posts)
	api.HandleFunc("GET /api/posts", h.posts)
	api.HandleFunc("GET /api/posts/{postID}", h.post)
	api.HandleFunc("PATCH /api/posts/{postID}", h.updatePost)
	api.HandleFunc("DELETE /api/posts/{postID}", h.deletePost)
	api.HandleFunc("POST /api/posts/{postID}/toggle_reaction", h.togglePostReaction)
	api.HandleFunc("POST /api/posts/{postID}/toggle_subscription", h.togglePostSubscription)
	api.HandleFunc("POST /api/timeline", h.createPost)
	api.HandleFunc("GET /api/timeline", h.timeline)
	api.HandleFunc("DELETE /api/timeline/{timelineItemID}", h.deleteTimelineItem)
	api.HandleFunc("POST /api/posts/{postID}/comments", h.createComment)
	api.HandleFunc("GET /api/posts/{postID}/comments", h.comments)
	api.HandleFunc("PATCH /api/comments/{commentID}", h.updateComment)
	api.HandleFunc("DELETE /api/comments/{commentID}", h.deleteComment)
	api.HandleFunc("POST /api/comments/{commentID}/toggle_reaction", h.toggleCommentReaction)
	api.HandleFunc("GET /api/notifications", h.notifications)
	api.HandleFunc("GET /api/has_unread_notifications", h.hasUnreadNotifications)
	api.HandleFunc("POST /api/notifications/{notificationID}/mark_as_read", h.markNotificationAsRead)
	api.HandleFunc("POST /api/mark_notifications_as_read", h.markNotificationsAsRead)
	api.HandleFunc("POST /api/web_push_subscriptions", h.addWebPushSubscription)

	proxy := withCacheControl(proxyCacheControl)(h.proxy)
	api.HandleFunc("HEAD /api/proxy", proxy)
	api.HandleFunc("GET /api/proxy", proxy)

	api.HandleFunc("POST /api/logs", h.pushLog)
	api.Handle("GET /api/prom", promHandler)

	r := http.NewServeMux()
	r.Handle("/api/", h.withAuth(api))
	r.Handle("/", h.staticHandler())

	return sess.LoadAndSave(r)
}
