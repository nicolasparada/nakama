package web

import (
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"
	tmplrenderer "github.com/nicolasparada/go-tmpl-renderer"
	"github.com/nicolasparada/nakama/service"
)

//go:embed templates/includes/*.tmpl templates/*.tmpl
var templatesFS embed.FS

type Handler struct {
	Service       *service.Service
	ErrorLogger   *slog.Logger
	SesssionStore scs.Store
	MinioURL      string

	renderer *tmplrenderer.Renderer
	sess     *scs.SessionManager
	handler  http.Handler
	once     sync.Once
}

func (h *Handler) init() {
	funcMap["minio"] = h.buildMinioURL
	h.renderer = &tmplrenderer.Renderer{
		FS:             templatesFS,
		BaseDir:        "templates",
		IncludePatters: []string{"includes/*.tmpl"},
		FuncMap:        funcMap,
	}
	h.sess = scs.New()
	h.sess.Store = h.SesssionStore
	h.sess.Lifetime = time.Hour * 24 * 7 // 7 days
	h.sess.ErrorFunc = func(w http.ResponseWriter, r *http.Request, err error) {
		h.renderErrorPage(w, r, fmt.Errorf("session error: %w", err))
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", h.showHome)
	mux.HandleFunc("GET /login", h.showLogin)
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("POST /logout", h.logout)
	mux.HandleFunc("GET /u/{username}", h.showUser)
	mux.HandleFunc("GET /u/{username}/edit", h.showEditUser)
	mux.HandleFunc("POST /user_avatars", h.uploadAvatar)
	mux.HandleFunc("POST /users/{userID}/toggle-follow", h.toggleFollow)
	mux.HandleFunc("POST /posts", h.createPost)
	mux.HandleFunc("GET /p/{postID}", h.showPost)
	mux.HandleFunc("POST /posts/{postID}/comments", h.createComment)
	mux.HandleFunc("GET /publications", h.showPublications)
	mux.HandleFunc("GET /publications/new", h.showCreatePublication)
	mux.HandleFunc("POST /publications", h.createPublication)
	mux.HandleFunc("GET /publications/{publicationID}", h.showPublication)
	mux.HandleFunc("GET /publications/{publicationID}/chapters/new", h.showCreateChapter)
	mux.HandleFunc("POST /publications/{publicationID}/chapters", h.createChapter)
	mux.HandleFunc("GET /notifications", h.notifications)
	mux.HandleFunc("POST /notifications/{notificationID}/read", h.readNotification)
	mux.HandleFunc("GET /search", h.search)
	mux.HandleFunc("POST /posts/{postID}/toggle-reaction", h.toggleReaction)
	mux.HandleFunc("POST /comments/{commentID}/toggle-reaction", h.toggleCommentReaction)
	mux.HandleFunc("GET /proxy", h.proxy)
	mux.Handle("GET /static/", staticHandler())
	mux.HandleFunc("GET /", h.notFound)

	h.handler = mux
	h.handler = h.withUser(h.handler)
	h.handler = h.sess.LoadAndSave(h.handler)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.once.Do(h.init)
	h.handler.ServeHTTP(w, r)
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.renderErrorPage(w, r, errPageNotFound)
}
